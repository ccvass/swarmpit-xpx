(ns swarmpit.docker.engine.http
  "Docker HTTP client using Java 21 native HttpClient with Unix domain sockets.
   No JNR, no Apache HttpClient, no clj-http for Docker calls."
  (:require [cheshire.core :refer [parse-string generate-string]]
            [clojure.string :as str]
            [swarmpit.config :refer [config]]
            [swarmpit.utils :refer [parse-int]]
            [taoensso.timbre :refer [error debug]])
  (:import (java.net URI UnixDomainSocketAddress)
           (java.net.http HttpClient HttpRequest HttpRequest$BodyPublishers
                          HttpResponse HttpResponse$BodyHandlers)
           (java.nio.channels SocketChannel)
           (java.time Duration)
           (java.io IOException ByteArrayOutputStream)))

(defn- http?
  [] (some? (re-matches #"^https?://.*" (config :docker-sock))))

(defn- url
  [uri unversioned?]
  (let [server (if (http?)
                 (config :docker-sock)
                 "http://localhost")
        api (if unversioned? "" (str "/v" (config :docker-api)))]
    (str server api uri)))

(defn- unix-request
  "Execute HTTP request over Unix domain socket using raw SocketChannel + HTTP/1.1"
  [{:keys [method uri body timeout-ms]}]
  (let [sock-path (config :docker-sock)
        addr (UnixDomainSocketAddress/of sock-path)
        channel (SocketChannel/open java.net.StandardProtocolFamily/UNIX)]
    (try
      (.connect channel addr)
      (.configureBlocking channel true)
      (let [method-str (name method)
            body-bytes (when body (.getBytes ^String body "UTF-8"))
            content-length (if body-bytes (count body-bytes) 0)
            request-line (str method-str " " uri " HTTP/1.0\r\n"
                              "Host: localhost\r\n"
                              "Accept: application/json\r\n"
                              "Content-Type: application/json\r\n"
                              "Content-Length: " content-length "\r\n"
                              "\r\n")
            request-bytes (.getBytes request-line "UTF-8")
            out-buf (java.nio.ByteBuffer/allocate (+ (count request-bytes) content-length))]
        (.put out-buf request-bytes)
        (when body-bytes (.put out-buf body-bytes))
        (.flip out-buf)
        (while (.hasRemaining out-buf)
          (.write channel out-buf))
        ;; Read full response (HTTP/1.0 + Connection: close = read until EOF)
        (let [chunks (java.io.ByteArrayOutputStream.)
              read-buf (java.nio.ByteBuffer/allocate 65536)]
          (loop []
            (let [n (.read channel read-buf)]
              (when (> n 0)
                (.write chunks (.array read-buf) 0 n)
                (.clear read-buf)
                (recur))))
          (let [response-str (.toString chunks "UTF-8")
                header-end (.indexOf response-str "\r\n\r\n")]
            (if (< header-end 0)
              {:status 502 :body "" :headers {}}
              (let [headers-str (subs response-str 0 header-end)
                    body-str (subs response-str (+ header-end 4))
                    status-line (first (.split headers-str "\r\n"))
                    status (try (Integer/parseInt (second (.split status-line " " 3)))
                                (catch Exception _ 500))]
                {:status status
                 :body body-str
                 :headers (into {}
                                (for [line (rest (.split headers-str "\r\n"))
                                      :let [idx (.indexOf line ": ")]
                                      :when (> idx 0)]
                                  [(keyword (.toLowerCase (subs line 0 idx)))
                                   (.trim (subs line (+ idx 2)))]))})))))
      (finally
        (.close channel)))))

(defn- tcp-request
  "Execute HTTP request over TCP using Java HttpClient"
  [{:keys [method uri body timeout-ms]}]
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
     :headers (into {}
                    (for [[k vs] (.map (.headers response))]
                      [(keyword k) (first vs)]))}))

(defn execute
  [{:keys [method api options unversioned?]}]
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
                          {:status status
                           :type :http-client
                           :body {:error (or (:message response-body) response-body)}}))
          {:status status
           :headers (:headers response)
           :body response-body}))
      (catch IOException e
        (error "Docker request failed:" (.getMessage e))
        (throw (ex-info (str "Docker failure: " (.getMessage e))
                        {:status 500
                         :type :http-client
                         :body {:error (.getMessage e)}}))))))
