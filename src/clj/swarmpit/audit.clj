(ns swarmpit.audit
  (:require [next.jdbc :as jdbc]
            [next.jdbc.result-set :as rs]
            [swarmpit.config :refer [config]]
            [clojure.tools.logging :as log])
  (:import [java.util UUID]))

(defn- ds []
  (jdbc/get-datasource {:dbtype "sqlite" :dbname (str (config :db-path) "/swarmpit.db")}))

(defn init-schema! []
  (jdbc/execute-one! (ds)
    ["CREATE TABLE IF NOT EXISTS audit_log (
       id TEXT PRIMARY KEY, timestamp INTEGER DEFAULT (strftime('%s','now')),
       username TEXT, action TEXT, resource_type TEXT, resource_name TEXT, details TEXT)"]))

(defn record! [username action resource-type resource-name]
  (jdbc/execute-one! (ds)
    ["INSERT INTO audit_log (id, username, action, resource_type, resource_name) VALUES (?,?,?,?,?)"
     (str (UUID/randomUUID)) username action resource-type resource-name]))

(defn list-entries [& {:keys [limit offset] :or {limit 50 offset 0}}]
  (jdbc/execute! (ds)
    ["SELECT * FROM audit_log ORDER BY timestamp DESC LIMIT ? OFFSET ?" limit offset]
    {:builder-fn rs/as-unqualified-kebab-maps}))

(defn wrap-audit [handler]
  (fn [request]
    (let [response (handler request)]
      (when (and (#{200 201 202} (:status response))
                 (#{:post :delete} (:request-method request)))
        (try
          (let [user (get-in request [:identity :usr :username] "system")
                method (name (:request-method request))
                uri (:uri request)]
            (record! user method "api" uri))
          (catch Exception e (log/debug "Audit:" (.getMessage e)))))
      response)))

(defn audit-list [{{:keys [query]} :parameters}]
  {:status 200
   :body (list-entries :limit (or (:limit query) 50) :offset (or (:offset query) 0))})
