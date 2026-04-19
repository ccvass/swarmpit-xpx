(ns swarmpit.health
  "Liveness and readiness probes"
  (:require [swarmpit.couchdb.client :as cc]
            [swarmpit.docker.engine.client :as dc]
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
  (let [db-ok     (check #(cc/version))
        docker-ok (check #(dc/ping))
        all-ok    (and db-ok docker-ok)]
    {:status (if all-ok 200 503)
     :body   {:status     (if all-ok "UP" "DOWN")
              :components {:sqlite   (if db-ok "UP" "DOWN")
                           :docker   (if docker-ok "UP" "DOWN")
                           :circuits {:docker (name ((:status res/docker-cb)))}
                           :stats    "in-memory"}}}))
