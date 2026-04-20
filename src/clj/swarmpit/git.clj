(ns swarmpit.git
  (:require [clojure.java.shell :as shell]
            [clojure.java.io :as io]
            [swarmpit.docker.engine.cli :as dcli]
            [swarmpit.config :refer [config]]
            [clojure.tools.logging :as log])
  (:import [java.util UUID]))

(defn- tmp-dir [] (str (config :work-dir) "/git-" (UUID/randomUUID)))

(defn- delete-dir [path]
  (let [f (io/file path)]
    (when (.exists f)
      (doseq [child (reverse (file-seq f))] (.delete child)))))

(defn deploy-from-git! [{:keys [repo-url branch compose-path stack-name]}]
  (let [dir (tmp-dir)
        branch (or branch "main")
        compose-path (or compose-path "docker-compose.yml")]
    (try
      (let [result (shell/sh "git" "clone" "--depth" "1" "-b" branch repo-url dir)]
        (when-not (zero? (:exit result))
          (throw (ex-info (str "Git clone failed: " (:err result))
                          {:status 400 :type :api :body {:error (:err result)}})))
        (let [compose-file (io/file dir compose-path)]
          (when-not (.exists compose-file)
            (throw (ex-info "Compose file not found"
                            {:status 400 :type :api :body {:error "Compose file not found"}})))
          (dcli/stack-deploy stack-name (slurp compose-file))
          {:stack stack-name :source repo-url :branch branch}))
      (finally (delete-dir dir)))))

(defn git-deploy [{{:keys [body]} :parameters}]
  (let [{:keys [repo-url stack-name]} body]
    (if (and repo-url stack-name)
      {:status 200 :body (deploy-from-git! body)}
      {:status 400 :body {:error "repo-url and stack-name required"}})))
