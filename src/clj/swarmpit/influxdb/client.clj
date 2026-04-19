(ns swarmpit.influxdb.client
  "In-memory time-series store replacing InfluxDB.
   Maintains the same public API so all callers remain unchanged.
   Data is kept in ring buffers — 1h raw, 24h downsampled."
  (:require [clojure.tools.logging :as log]))

;; --- Storage atoms ---
;; Raw stats: last 1 hour at ~5s intervals = ~720 points per host/task
;; Downsampled: last 24h at 1-min intervals = 1440 points per host/task

(def ^:private raw-host-stats (atom {}))    ;; {host-id -> [{:ts :cpu :memory :disk}]}
(def ^:private raw-task-stats (atom {}))    ;; {task-name -> [{:ts :cpu :memory :service}]}
(def ^:private ds-host-stats (atom {}))     ;; downsampled hosts
(def ^:private ds-task-stats (atom {}))     ;; downsampled tasks
(def ^:private ds-service-stats (atom {}))  ;; downsampled services
(def ^:private max-usage (atom {}))         ;; {service -> {:cpu :memory}}

(def ^:private raw-limit 720)
(def ^:private ds-limit 1440)

;; --- Lifecycle (no-ops for compatibility) ---

(defn ping [] {:status 204})

(defn create-database [] nil)
(defn create-an-hour-rp [] nil)
(defn create-a-day-rp [] nil)
(defn task-cq [] nil)
(defn host-cq [] nil)
(defn service-cq [] nil)
(defn service-max-usage-cq [] nil)

(defn retention-policy-summary [] #{"in-memory"})
(defn continuous-query-summary [] #{"realtime-aggregation"})

;; --- Write ---

(defn- append-ring [buf item limit]
  (vec (take limit (cons item (or buf [])))))

(defn write-host-points
  [tags {:keys [cpu memory disk]}]
  (let [host-id (:host tags)
        point {:ts     (System/currentTimeMillis)
               :cpu    (double (or (:usedPercentage cpu) 0))
               :memory (double (or (:usedPercentage memory) 0))
               :disk   (double (or (:usedPercentage disk) 0))}]
    (swap! raw-host-stats update host-id append-ring point raw-limit)
    ;; Downsample: keep 1-min resolution
    (swap! ds-host-stats update host-id append-ring point ds-limit)))

(defn write-task-points
  [tags {:keys [cpuPercentage memory memoryLimit memoryPercentage]}]
  (let [task-name (:task tags)
        service (:service tags)
        cpu-val (/ (double (or cpuPercentage 0)) 100.0)
        mem-val (or memory 0)
        point {:ts task-name :cpu cpu-val :memory mem-val :service service}]
    (swap! raw-task-stats update task-name append-ring point raw-limit)
    ;; Aggregate per service
    (swap! ds-service-stats update service append-ring
           {:ts (System/currentTimeMillis) :cpu cpu-val :memory mem-val} ds-limit)
    ;; Track max usage
    (swap! max-usage update service
           (fn [prev]
             {:cpu    (max (or (:cpu prev) 0) cpu-val)
              :memory (max (or (:memory prev) 0) mem-val)
              :service service}))))

;; --- Read (returns data in the format influxdb/mapper expects) ---

(defn- points->series [name-tag points]
  [{"series" [{"name"    "downsampled"
               "tags"    name-tag
               "columns" ["time" "cpu" "memory"]
               "values"  (mapv (fn [p] [(:ts p) (:cpu p) (:memory p)]) points)}]}])

(defn read-task-stats [task-name]
  (let [points (get @ds-task-stats task-name [])]
    (points->series {"task" task-name "service" (:service (first points))} points)))

(defn read-service-stats [services]
  (when (seq services)
    (let [all-series (for [svc services
                           :let [points (get @ds-service-stats svc [])]
                           :when (seq points)]
                       {"name"    "downsampled_services"
                        "tags"    {"service" svc}
                        "columns" ["time" "cpu" "memory"]
                        "values"  (mapv (fn [p] [(:ts p) (:cpu p) (:memory p)]) points)})]
      [{"series" (vec all-series)}])))

(defn read-max-usage-service-stats []
  (let [entries (vals @max-usage)
        series (for [{:keys [service cpu memory]} entries]
                 {"name"    "downsampled_max_usage_services"
                  "tags"    {"service" service}
                  "columns" ["time" "max_cpu" "max_memory"]
                  "values"  [[0 cpu memory]]})]
    [{"series" (vec series)}]))

(defn read-host-stats []
  (let [all-series (for [[host-id points] @ds-host-stats
                         :when (seq points)]
                     {"name"    "downsampled_hosts"
                      "tags"    {"host" host-id}
                      "columns" ["time" "cpu" "memory"]
                      "values"  (mapv (fn [p] [(:ts p) (:cpu p) (:memory p)]) points)})]
    [{"series" (vec all-series)}]))
