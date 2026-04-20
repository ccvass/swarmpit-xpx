(ns swarmpit.exec
  (:require [org.httpkit.server :refer [with-channel on-close on-receive send!]]
            [swarmpit.docker.engine.http :as docker-http]
            [cheshire.core :refer [generate-string parse-string]]
            [clojure.tools.logging :as log])
  (:import [java.net UnixDomainSocketAddress]
           [java.nio.channels SocketChannel]
           [java.nio ByteBuffer]
           [java.nio.charset StandardCharsets]))

(defn- create-exec [container-id cmd]
  (docker-http/execute
    {:method :POST
     :api    (str "/containers/" container-id "/exec")
     :options {:body {:AttachStdin true :AttachStdout true :AttachStderr true
                      :Tty true :Cmd (or cmd ["/bin/sh"])}}}))

(defn- start-exec-stream
  "Connect raw to Docker exec start endpoint, return SocketChannel"
  [exec-id]
  (let [sock-path (swarmpit.config/config :docker-sock)
        channel (SocketChannel/open java.net.StandardProtocolFamily/UNIX)
        _ (.connect channel (UnixDomainSocketAddress/of sock-path))
        request (str "POST /exec/" exec-id "/start HTTP/1.1\r\n"
                     "Host: localhost\r\n"
                     "Content-Type: application/json\r\n"
                     "Connection: Upgrade\r\n"
                     "Upgrade: tcp\r\n"
                     "Content-Length: 7\r\n\r\n"
                     "{\"Tty\":true}")
        buf (ByteBuffer/wrap (.getBytes request StandardCharsets/UTF_8))]
    (while (.hasRemaining buf) (.write channel buf))
    ;; Skip HTTP response headers
    (let [hdr-buf (ByteBuffer/allocate 4096)
          _ (.read channel hdr-buf)]
      (.configureBlocking channel false))
    channel))

(defn exec-handler
  "WebSocket handler for container exec"
  [{{:keys [path query]} :parameters :as request}]
  (with-channel request ws-ch
    (let [container-id (:id path)
          cmd (when-let [c (:cmd query)] [c])
          exec-resp (create-exec container-id cmd)
          exec-id (get-in exec-resp [:body :Id])]
      (if-not exec-id
        (send! ws-ch (str "Error: cannot create exec") true)
        (let [docker-ch (start-exec-stream exec-id)]
          ;; Read from Docker → send to WebSocket
          (future
            (try
              (let [buf (ByteBuffer/allocate 4096)]
                (loop []
                  (when (.isOpen docker-ch)
                    (.clear buf)
                    (let [n (.read docker-ch buf)]
                      (when (and n (> n 0))
                        (.flip buf)
                        (let [data (String. (.array buf) 0 n StandardCharsets/UTF_8)]
                          (send! ws-ch data false))
                        (recur))))))
              (catch Exception e
                (log/debug "Exec read ended:" (.getMessage e)))
              (finally (.close docker-ch))))
          ;; Write from WebSocket → Docker
          (on-receive ws-ch
                      (fn [data]
                        (try
                          (let [buf (ByteBuffer/wrap (.getBytes (str data) StandardCharsets/UTF_8))]
                            (while (.hasRemaining buf) (.write docker-ch buf)))
                          (catch Exception _ nil))))
          (on-close ws-ch
                    (fn [_]
                      (try (.close docker-ch) (catch Exception _ nil)))))))))
