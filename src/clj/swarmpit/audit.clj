(ns swarmpit.audit
  (:require [next.jdbc :as jdbc]
            [next.jdbc.result-set :as rs]
            [swarmpit.handler :refer [resp-ok]]
            [clojure.tools.logging :as log])
  (:import [java.util UUID]))

(defn- conn [] @(resolve 'swarmpit.couchdb.client/ds))

(defn init-schema! []
  (jdbc/execute-one! (conn)
    ["CREATE TABLE IF NOT EXISTS audit_log (
       id TEXT PRIMARY KEY, timestamp INTEGER DEFAULT (strftime('%s','now')),
       username TEXT, action TEXT, resource_type TEXT, resource_name TEXT, details TEXT)"]))

(defn record! [username action resource-type resource-name]
  (jdbc/execute-one! (conn)
    ["INSERT INTO audit_log (id, username, action, resource_type, resource_name) VALUES (?,?,?,?,?)"
     (str (UUID/randomUUID)) username action resource-type resource-name]))

(defn list-entries [& {:keys [limit offset] :or {limit 50 offset 0}}]
  (jdbc/execute! (conn)
    ["SELECT * FROM audit_log ORDER BY timestamp DESC LIMIT ? OFFSET ?" limit offset]
    {:builder-fn rs/as-unqualified-kebab-maps}))

;; Middleware

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
          (catch Exception e
            (log/debug "Audit record failed:" (.getMessage e)))))
      response)))

;; Handler

(defn audit-list
  [{{:keys [query]} :parameters}]
  (resp-ok (list-entries :limit (or (:limit query) 50)
                         :offset (or (:offset query) 0))))
