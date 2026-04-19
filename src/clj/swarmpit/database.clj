(ns swarmpit.database
  (:require [swarmpit.couchdb.client :as cc]
            [swarmpit.couchdb.migration :refer [migrate]]
            [taoensso.timbre :refer [info error]]))

(defn init
  []
  (info "Initializing embedded SQLite database...")
  (try
    (if (cc/database-exist?)
      (info "Swarmpit DB already exists")
      (do
        (cc/create-database)
        (info "Swarmpit DB created")))
    (migrate)
    (info "Database ready (SQLite embedded)")
    (catch Exception e
      (error "Database init failed:" (.getMessage e))
      (throw e))))
