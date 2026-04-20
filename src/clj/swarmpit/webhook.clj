(ns swarmpit.webhook
  (:require [next.jdbc :as jdbc]
            [next.jdbc.result-set :as rs]
            [swarmpit.docker.engine.client :as dc]
            [clojure.tools.logging :as log])
  (:import [java.util UUID]))

(defn- conn [] @@(resolve (quote swarmpit.couchdb.client/ds)))
(defn- ok ([] {:status 200}) ([b] {:status 200 :body b}))
(defn- err [s m] {:status s :body {:error m}})

(defn init-schema! []
  (jdbc/execute-one! (conn)
    ["CREATE TABLE IF NOT EXISTS webhooks (
       id TEXT PRIMARY KEY, service_id TEXT NOT NULL, token TEXT UNIQUE NOT NULL,
       created_at INTEGER DEFAULT (strftime('%s','now')), last_triggered INTEGER)"]))

(defn create-webhook [service-id]
  (let [token (str (UUID/randomUUID)) id (str (UUID/randomUUID))]
    (jdbc/execute-one! (conn)
      ["INSERT INTO webhooks (id, service_id, token) VALUES (?, ?, ?)" id service-id token])
    {:token token :service-id service-id}))

(defn delete-webhook [token]
  (jdbc/execute-one! (conn) ["DELETE FROM webhooks WHERE token = ?" token]))

(defn- find-by-token [token]
  (jdbc/execute-one! (conn)
    ["SELECT * FROM webhooks WHERE token = ?" token]
    {:builder-fn rs/as-unqualified-kebab-maps}))

(defn trigger [token]
  (when-let [wh (find-by-token token)]
    (try
      (let [svc (dc/service (:service-id wh))
            spec (:Spec svc)
            version (get-in svc [:Version :Index])]
        (dc/update-service (:service-id wh) version
          (assoc-in spec [:TaskTemplate :ForceUpdate]
                    (inc (or (get-in spec [:TaskTemplate :ForceUpdate]) 0))))
        (jdbc/execute-one! (conn)
          ["UPDATE webhooks SET last_triggered = strftime('%s','now') WHERE token = ?" token])
        true)
      (catch Exception e (log/warn "Webhook trigger failed:" (.getMessage e)) nil))))

(defn webhook-trigger [{{:keys [path]} :parameters}]
  (if (trigger (:token path)) (ok {:status "triggered"}) (err 404 "Webhook not found")))

(defn webhook-create [{{:keys [body]} :parameters}]
  (ok (create-webhook (:service-id body))))

(defn webhook-delete [{{:keys [path]} :parameters}]
  (delete-webhook (:token path)) (ok))
