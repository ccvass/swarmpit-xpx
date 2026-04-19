(ns swarmpit.ratelimit
  (:require [clojure.core.cache :as cache]))

(def ^:private login-attempts
  (atom (cache/ttl-cache-factory {} :ttl 60000)))

(defn- client-ip [request]
  (or (get-in request [:headers "x-forwarded-for"])
      (get-in request [:headers "x-real-ip"])
      (:remote-addr request)))

(defn- rate-limited? [ip limit]
  (let [current (cache/lookup @login-attempts ip)]
    (and current (>= current limit))))

(defn- record-attempt! [ip]
  (swap! login-attempts
         (fn [c]
           (let [current (or (cache/lookup c ip) 0)]
             (cache/miss c ip (inc current))))))

(defn wrap-login-ratelimit
  "Ring middleware: limits login attempts to 5 per minute per IP"
  [handler]
  (fn [request]
    (let [ip (client-ip request)]
      (if (rate-limited? ip 5)
        {:status  429
         :headers {"Retry-After" "60"}
         :body    {:error "Too many login attempts. Try again in 60 seconds."}}
        (do (record-attempt! ip)
            (handler request))))))
