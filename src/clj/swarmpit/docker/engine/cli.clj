(ns swarmpit.docker.engine.cli
  (:require [clojure.java.shell :as shell]
            [clojure.java.io :refer [make-parents delete-file]]
            [clojure.string :as str]
            [cheshire.core :refer [parse-string]]
            [swarmpit.config :refer [config]]))

(defn- sanitize-name
  "Remove path-traversal and shell-unsafe characters from stack name"
  [name]
  (when name
    (str/replace name #"[^a-zA-Z0-9_\-\.]" "")))

(defn- execute
  "Execute docker command and parse result"
  ([cmd]
   (execute cmd nil))
  ([cmd stdin]
   (let [result (if stdin
                  (apply shell/sh (concat cmd [:in stdin]))
                  (apply shell/sh cmd))]
     (if (= 0 (:exit result))
       {:result (:out result)}
       (throw
         (let [error (:err result)]
           (ex-info (str "Docker error: " error)
                    {:status 400
                     :type   :docker-cli
                     :body   {:error error}})))))))

(defn- login-cmd
  [username server]
  (cond-> ["docker" "login" "--username" username "--password-stdin"]
    (not (str/blank? server)) (conj server)))

(defn- stack-deploy-cmd
  [name file {:keys [skip-resolve-image]}]
  (cond-> ["docker" "stack" "deploy" "--with-registry-auth" "--compose-file" file]
    skip-resolve-image (conj "--resolve-image" "never")
    true               (conj name)))

(defn- stack-remove-cmd
  [name]
  ["docker" "stack" "rm" name])

(defn- stack-file
  [name]
  (let [safe-name (sanitize-name name)]
    (when (str/blank? safe-name)
      (throw (ex-info "Invalid stack name"
                      {:status 400 :type :api
                       :body {:error "Stack name must contain alphanumeric characters"}})))
    (str (config :work-dir) "/" safe-name ".yml")))

(defn login
  [username password server]
  (-> (login-cmd username server)
      (execute password)))

(defn stack-deploy
  [name compose & [{:keys [skip-resolve-image] :as opts}]]
  (let [safe-name (sanitize-name name)
        file (stack-file name)
        cmd (stack-deploy-cmd safe-name file opts)]
    (try
      (make-parents file)
      (spit file compose)
      (execute cmd)
      (finally
        (delete-file file)))))

(defn stack-remove
  [name]
  (let [safe-name (sanitize-name name)
        cmd (stack-remove-cmd safe-name)]
    (execute cmd)))
