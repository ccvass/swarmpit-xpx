(ns swarmpit.resilience
  "Circuit breaker and health check infrastructure"
  (:require [clojure.tools.logging :as log]))

(defn circuit-breaker
  "Creates a circuit breaker that opens after `threshold` failures within
   `window-ms`, stays open for `cooldown-ms` before half-open retry."
  [{:keys [threshold window-ms cooldown-ms name]
    :or   {threshold 5 window-ms 10000 cooldown-ms 30000 name "unknown"}}]
  (let [state (atom {:failures [] :open-since nil})]
    {:call   (fn [f fallback]
               (let [{:keys [failures open-since]} @state
                     now (System/currentTimeMillis)]
                 (cond
                   ;; Open and cooling down
                   (and open-since (< (- now open-since) cooldown-ms))
                   (do (log/debug "Circuit" name "is OPEN, using fallback")
                       (fallback))

                   ;; Half-open: cooldown elapsed, try once
                   open-since
                   (try
                     (let [result (f)]
                       (reset! state {:failures [] :open-since nil})
                       (log/info "Circuit" name "CLOSED (recovered)")
                       result)
                     (catch Exception e
                       (swap! state assoc :open-since now)
                       (log/warn "Circuit" name "remains OPEN:" (.getMessage e))
                       (fallback)))

                   ;; Closed: normal operation
                   :else
                   (try
                     (f)
                     (catch Exception e
                       (let [recent (filterv #(< (- now %) window-ms) failures)
                             updated (conj recent now)]
                         (if (>= (count updated) threshold)
                           (do (reset! state {:failures [] :open-since now})
                               (log/warn "Circuit" name "OPENED after" threshold "failures"))
                           (swap! state assoc :failures updated)))
                       (throw e))))))
     :status (fn [] (if (:open-since @state) :open :closed))
     :reset  (fn [] (reset! state {:failures [] :open-since nil}))}))

;; Application circuit breakers
(def docker-cb
  (circuit-breaker {:threshold 5 :window-ms 10000 :cooldown-ms 30000 :name "docker"}))

(def couch-cb
  (circuit-breaker {:threshold 3 :window-ms 5000 :cooldown-ms 15000 :name "couchdb"}))

(def influx-cb
  (circuit-breaker {:threshold 3 :window-ms 10000 :cooldown-ms 60000 :name "influxdb"}))
