(ns swarmpit.event.channel
  (:refer-clojure :exclude [list])
  (:require [org.httpkit.server :refer [send!]]
            [clojure.core.async :refer [go <! timeout]]
            [clojure.edn :as edn]
            [clojure.walk :refer [keywordize-keys]]
            [cheshire.core :refer [generate-string]]
            [swarmpit.base64 :as base64]
            [swarmpit.event.rules.subscription :as rule]
            [swarmpit.event.rules.subscription-stats :as stats]))

(def hub (atom {}))

(defn- event-data
  [data]
  (str "data: " (generate-string data) "\n\n"))

(defn- subscription
  [channel]
  (-> (val channel)
      (get-in [:query-params "subscription"])
      (base64/decode)
      (edn/read-string)))

(defn- subscription-user
  [channel]
  (-> (val channel) :identity))

(defn- subscribers
  ([channel-subscription]
   "Get subscribers based on given subscription"
   (->> @hub
        (filter #(= channel-subscription (subscription %)))))
  ([]
   "Get all subscribers"
   (-> @hub)))

(defn list
  ([{:keys [type message] :as event}]
   "Get subscribed channels based on given event"
   (->> (filter #(rule/match? % type message) rule/list)
        (map #(rule/subscription % message))
        (map #(subscribers %))
        (flatten)
        (filter #(some? %))
        (map #(str %))))
  ([]
   "Get subscribed channels"
   (->> (keys @hub)
        (map #(str %)))))

(defn broadcast
  [{:keys [type message] :as event}]
  (go
    (<! (timeout 1000))
    (doseq [rule (filter #(rule/match? % type message) rule/list)]
      (let [subscription (rule/subscription rule message)
            subs (subscribers subscription)]
        (doseq [subscriber subs]
          (let [user (subscription-user subscriber)
                data (rule/subscribed-data rule message user)
                ch (key subscriber)]
            (when-not (send! ch (event-data data) false)
              (swap! hub dissoc ch))))))))

(defn broadcast-statistics
  []
  (go
    (let [subs (filter #(contains? stats/subscribers (:handler (subscription %))) @hub)]
      (doseq [subscriber subs]
        (let [user (subscription-user subscriber)
              sub (subscription subscriber)
              data (stats/subscribed-data sub user)
              ch (key subscriber)]
          (when-not (send! ch (event-data data) false)
            (swap! hub dissoc ch)))))))

;; To prevent data duplicity/spam we debounce:
;;
;; 1) Swarm scoped events that are received from each manager at the same time
;; 2) Same local scoped events that occured within 1 second
(def ^:private last-broadcast (atom {}))

(defn broadcast-memo
  "Debounced broadcast — ignores duplicate events within 1 second"
  [event]
  (let [k (hash event)
        now (System/currentTimeMillis)
        prev (get @last-broadcast k 0)]
    (when (> (- now prev) 1000)
      (swap! last-broadcast assoc k now)
      (broadcast event))))