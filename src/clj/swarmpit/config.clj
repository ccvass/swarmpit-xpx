(ns swarmpit.config
  (:require [environ.core :refer [env]]))

(def default
  (atom {:docker-sock         "/var/run/docker.sock"
         :docker-api          "1.44"
         :docker-http-timeout 5000
         :log-level           "info"
         :db-path             "/data"
         :agent-url           nil
         :work-dir            "/tmp"
         :instance-name       nil
         :password-hashing    {:alg        :pbkdf2+sha512
                               :iterations 200000}}))

(def environment
  (->> {:docker-sock         (env :swarmpit-docker-sock)
        :docker-api          (env :swarmpit-docker-api)
        :docker-http-timeout (env :swarmpit-docker-http-timeout)
        :log-level           (env :swarmpit-log-level)
        :db-path             (env :swarmpit-db-path)
        :agent-url           (env :swarmpit-agent-url)
        :work-dir            (env :swarmpit-workdir)
        :instance-name       (env :swarmpit-instance-name)}
       (into {} (remove #(nil? (val %))))))

(def ^:private dynamic (atom {}))

(defn update!
  [config] (reset! dynamic config))

(defn config
  ([] (->> [@default environment @dynamic]
           (apply merge)))
  ([key] ((config) key)))
