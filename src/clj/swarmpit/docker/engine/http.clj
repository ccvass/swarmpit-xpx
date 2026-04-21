(ns swarmpit.docker.engine.http
  "Docker HTTP client with Unix socket connection pool."
  (:require [cheshire.core :refer [parse-string generate-string]]
            [clojure.string :as str]
            [swarmpit.config :refer [config]]
            [swarmpit.utils :refer [parse-int]]
            [taoensso.timbre :refer [error debug]])
  (:import (java.net URI UnixDomainSocketAddress)
           (java.net.http HttpClient HttpRequest HttpRequest$BodyPublishers
                          HttpResponse HttpResponse$BodyHandlers)
           (java.nio ByteBuffer)
           (java.nio.channels SocketChannel)
           (java.nio.charset StandardCharsets)
           (java.time Duration)
           (java.io IOException ByteArrayOutputStream)
           (java.util.concurrent LinkedBlockingDeque TimeUnit)))

(defn- http? []
  (some? (re-matches #"^https?://.*" (config :docker-sock))))

;; --- Unix socket connection pool ---

(def ^:private pool-size 16)
(def ^:private pool (LinkedBlockingDeque. pool-size))

(defn- new-channel []
  (let [ch (SocketChannel/open java.net.StandardProtocolFamily/UNIX)]
    (.connect ch (UnixDomainSocketAddress/of (config :docker-sock)))
    (.configureBlocking ch true)
    ch))

(defn- acquire-channel []
  (or (.poll pool)
      (new-channel)))

(defn- release-channel [ch]
  (if (and ch (.isOpen ch) (not (.offer pool ch)))
    (.close ch)))

(defn- discard-channel [ch]
  (when ch (try (.close ch) (catch Exception _))))

;; --- HTTP/1.1 over Unix socket with keep-alive ---

(defn- parse-response [^String raw]
  (let [header-end (.indexOf raw "\r\n\r\n")]
    (if (< header-end 0)
      {:status 502 :body "" :headers {}}
      (let [headers-str (subs raw 0 header-end)
            body-str (subs raw (+ header-end 4))
            status-line (first (.split headers-str "\r\n"))
            status (try (Integer/parseInt (second (.split status-line " " 3)))
                        (catch Exception _ 500))
            headers (into {}
                         (for [line (rest (.split headers-str "\r\n"))
                               :let [idx (.indexOf line ": ")]
                               :when (> idx 0)]
                           [(.toLowerCase (subs line 0 idx))
                            (.trim (subs line (+ idx 2)))]))]
        {:status status :body body-str :headers headers}))))

(defn- read-http-response
  "Read HTTP/1.1 response: parse headers, then read body by Content-Length or until connection close"
  [^SocketChannel ch timeout-ms]
  (let [buf (ByteBuffer/allocate 65536)
        out (ByteArrayOutputStream.)
        deadline (+ (System/currentTimeMillis) timeout-ms)]
    ;; Read until we have full headers + body
    (loop []
      (when (< (System/currentTimeMillis) deadline)
        (.clear buf)
        (let [n (.read ch buf)]
          (when (> n 0)
            (.write out (.array buf) 0 n)
            (let [so-far (.toString out "UTF-8")
                  hdr-end (.indexOf so-far "\r\n\r\n")]
              (if (>= hdr-end 0)
                ;; Have headers — check if we have full body
                (let [headers-str (subs so-far 0 hdr-end)
                      cl-match (re-find #"(?i)content-length:\s*(\d+)" headers-str)]
                  (if cl-match
                    (let [content-length (Integer/parseInt (second cl-match))
                          body-start (+ hdr-end 4)
                          body-so-far (- (.size out) body-start)]
                      (when (< body-so-far content-length)
                        (recur)))
                    ;; No content-length — chunked or close. Read one more attempt.
                    (do (.clear buf)
                        (let [n2 (.read ch buf)]
                          (when (> n2 0)
                            (.write out (.array buf) 0 n2))))))
                (recur)))))))
    (.toString out "UTF-8")))

(defn- unix-request [{:keys [method uri body timeout-ms]}]
  (let [ch (acquire-channel)]
    (try
      (let [method-str (name method)
            body-bytes (when body (.getBytes ^String body "UTF-8"))
            content-length (if body-bytes (count body-bytes) 0)
            req-str (str method-str " " uri " HTTP/1.1\r\n"
                         "Host: localhost\r\n"
                         "Accept: application/json\r\n"
                         "Content-Type: application/json\r\n"
                         "Content-Length: " content-length "\r\n"
                         "Connection: keep-alive\r\n"
                         "\r\n")
            req-bytes (.getBytes req-str "UTF-8")
            out-buf (ByteBuffer/allocate (+ (count req-bytes) content-length))]
        (.put out-buf req-bytes)
        (when body-bytes (.put out-buf body-bytes))
        (.flip out-buf)
        (while (.hasRemaining out-buf) (.write ch out-buf))
        (let [raw (read-http-response ch timeout-ms)
              response (parse-response raw)]
          (release-channel ch)
          response))
      (catch Exception e
        (discard-channel ch)
        (throw e)))))

;; --- TCP ---

(defn- tcp-request [{:keys [method uri body timeout-ms]}]
  (let [client (-> (HttpClient/newBuilder)
                   (.connectTimeout (Duration/ofMillis timeout-ms))
                   (.build))
        body-pub (if body
                   (HttpRequest$BodyPublishers/ofString body)
                   (HttpRequest$BodyPublishers/noBody))
        request (-> (HttpRequest/newBuilder)
                    (.uri (URI/create uri))
                    (.timeout (Duration/ofMillis timeout-ms))
                    (.header "Accept" "application/json")
                    (.header "Content-Type" "application/json")
                    (.method (name method) body-pub)
                    (.build))
        response (.send client request (HttpResponse$BodyHandlers/ofString))]
    {:status (.statusCode response)
     :body (.body response)
     :headers (into {} (for [[k vs] (.map (.headers response))] [(keyword k) (first vs)]))}))

;; --- Public API ---

(defn- url [uri unversioned?]
  (let [server (if (http?) (config :docker-sock) "http://localhost")
        api (if unversioned? "" (str "/v" (config :docker-api)))]
    (str server api uri)))

(defn execute [{:keys [method api options unversioned?]}]
  (let [timeout-ms (or (parse-int (config :docker-http-timeout)) 15000)
        uri (url api unversioned?)
        body (when-let [b (:body options)]
               (if (string? b) b (generate-string b)))
        query-params (:query-params options)
        full-uri (if query-params
                   (str uri "?" (str/join "&"
                                  (map (fn [[k v]] (str (name k) "=" (java.net.URLEncoder/encode (str v) "UTF-8")))
                                       query-params)))
                   uri)]
    (try
      (let [response (if (http?)
                       (tcp-request {:method method :uri full-uri :body body :timeout-ms timeout-ms})
                       (unix-request {:method method :uri full-uri :body body :timeout-ms timeout-ms}))
            status (:status response)
            response-body (try (parse-string (:body response) true)
                               (catch Exception _ (:body response)))]
        (if (>= status 400)
          (throw (ex-info (str "Docker error: " (or (:message response-body) response-body))
                          {:status status :type :http-client
                           :body {:error (or (:message response-body) response-body)}}))
          {:status status :headers (:headers response) :body response-body}))
      (catch IOException e
        (error "Docker request failed:" (.getMessage e))
        (throw (ex-info (str "Docker failure: " (.getMessage e))
                        {:status 500 :type :http-client :body {:error (.getMessage e)}}))))))
