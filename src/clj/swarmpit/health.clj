(ns swarmpit.health
  "Liveness and readiness probes"
  (:require [swarmpit.couchdb.client :as cc]
            [swarmpit.docker.engine.client :as dc]
            [swarmpit.influxdb.client :as ic]
            [swarmpit.config :refer [config]]
            [swarmpit.resilience :as res]))

(defn- check [f]
  (try (f) true (catch Exception _ false)))

(defn live
  "Liveness probe — is the JVM alive and serving?"
  [_]
  {:status 200
   :body   {:status "UP"}})

(defn ready
  "Readiness probe — are critical dependencies reachable?"
  [_]
  (let [couch-ok  (check #(cc/version))
        docker-ok (check #(dc/ping))
        influx-ok (or (nil? (config :influxdb-url))
                      (check #(ic/ping)))
        all-ok    (and couch-ok docker-ok)]
    {:status (if all-ok 200 503)
     :body   {:status     (if all-ok "UP" "DOWN")
              :components {:couchdb  (if couch-ok "UP" "DOWN")
                           :docker   (if docker-ok "UP" "DOWN")
                           :influxdb (if influx-ok "UP" "DOWN")
                           :circuits {:docker  (name ((:status res/docker-cb)))
                                      :couchdb (name ((:status res/couch-cb)))
                                      :influx  (name ((:status res/influx-cb)))}}}}))
