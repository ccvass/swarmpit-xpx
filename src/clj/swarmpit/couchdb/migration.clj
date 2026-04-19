(ns swarmpit.couchdb.migration
  (:require [swarmpit.couchdb.client :as db]
            [swarmpit.uuid :refer [uuid]]))

(defn- create-secret
  []
  (db/create-secret {:secret (uuid)})
  (println "Default token secret created"))

(defn- verify-initial-data
  []
  (when (nil? (db/get-secret))
    (create-secret)))

(def migrations
  {:initial verify-initial-data})

(defn migrate
  []
  (doseq [migration (->> (apply dissoc migrations (db/migrations))
                         (into []))]
    (db/record-migration (key migration) ((val migration)))))
