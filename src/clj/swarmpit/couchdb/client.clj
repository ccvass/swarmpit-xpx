(ns swarmpit.couchdb.client
  "Document store backed by embedded SQLite.
   Maintains the same public API as the original CouchDB client
   so all callers remain unchanged."
  (:require [next.jdbc :as jdbc]
            [next.jdbc.result-set :as rs]
            [cheshire.core :as json]
            [clojure.string :as str]
            [swarmpit.config :refer [config]])
  (:import [java.util UUID]))

;; --- Datasource ---

(defn- db-path []
  (str (or (config :db-path) "/data") "/swarmpit.db"))

(def ^:private ds
  (delay (jdbc/get-datasource {:dbtype "sqlite" :dbname (db-path)})))

(defn- conn [] @ds)

;; --- Schema ---

(def ^:private schema-ddl
  ["CREATE TABLE IF NOT EXISTS documents (
      id TEXT PRIMARY KEY,
      type TEXT NOT NULL,
      data TEXT NOT NULL
    )"
   "CREATE INDEX IF NOT EXISTS idx_doc_type ON documents(type)"
   "CREATE INDEX IF NOT EXISTS idx_doc_type_json ON documents(type, json_extract(data, '$.username'))"])

(defn create-database []
  (doseq [ddl schema-ddl]
    (jdbc/execute-one! (conn) [ddl])))

(defn database-exist? []
  (some? (jdbc/execute-one! (conn) ["SELECT name FROM sqlite_master WHERE type='table' AND name='documents'"])))

(defn version []
  {:sqlite (-> (jdbc/execute-one! (conn) ["SELECT sqlite_version() as v"]) :v)})

;; --- Core document operations ---

(defn- gen-id [] (str (UUID/randomUUID)))

(defn- row->doc [{:keys [id type data]}]
  (when data
    (-> (json/parse-string data true)
        (assoc :_id id :type type))))

(defn- doc->json [doc]
  (json/generate-string (dissoc doc :_id :_rev :type)))

(defn get-doc [id]
  (when-not (str/blank? id)
    (some-> (jdbc/execute-one! (conn)
              ["SELECT id, type, data FROM documents WHERE id = ?" id]
              {:builder-fn rs/as-unqualified-kebab-maps})
            row->doc)))

(defn create-doc [doc]
  (let [id (gen-id)
        t (:type doc)
        data (doc->json doc)]
    (jdbc/execute-one! (conn)
      ["INSERT INTO documents (id, type, data) VALUES (?, ?, ?)" id t data])
    {:ok true :id id :rev "1"}))

(defn update-doc
  ([doc]
   (let [id (:_id doc)
         t (:type doc)
         data (doc->json doc)]
     (jdbc/execute-one! (conn)
       ["UPDATE documents SET data = ?, type = ? WHERE id = ?" data t id])
     {:ok true :id id :rev "1"}))
  ([doc delta]
   (update-doc (merge doc delta)))
  ([doc field value]
   (update-doc (assoc doc field value))))

(defn delete-doc [doc]
  (jdbc/execute-one! (conn)
    ["DELETE FROM documents WHERE id = ?" (:_id doc)])
  {:ok true})

;; --- Query helpers ---

(defn find-docs
  ([type]
   (find-docs nil type))
  ([_query type]
   (->> (jdbc/execute! (conn)
          ["SELECT id, type, data FROM documents WHERE type = ?" type]
          {:builder-fn rs/as-unqualified-kebab-maps})
        (mapv row->doc))))

(defn find-doc [_query type]
  (first (find-docs _query type)))

(defn find-cross-docs [_query]
  ;; Used only for user-registries: find all registries owned by a user
  ;; The query structure is {"$and" [{:type {"$in" [types]}} {:owner {"$eq" username}}]}
  (let [types (get-in _query ["$and" 0 :type "$in"]
                      (get-in _query (list "$and" 0 :type "$in") []))
        owner (get-in _query ["$and" 1 :owner "$eq"]
                      (get-in _query (list "$and" 1 :owner "$eq") nil))]
    (if (and (seq types) owner)
      (->> (jdbc/execute! (conn)
             [(str "SELECT id, type, data FROM documents WHERE type IN ("
                   (str/join "," (repeat (count types) "?"))
                   ") AND json_extract(data, '$.owner') = ?")
              types owner]
             {:builder-fn rs/as-unqualified-kebab-maps})
           (mapv row->doc))
      [])))

;; --- Specialized finders (override find-docs with JSON queries) ---
;; These re-implement the Mango-style queries using SQLite JSON functions

(defn- find-by-field [type field value]
  (some-> (jdbc/execute-one! (conn)
            [(str "SELECT id, type, data FROM documents WHERE type = ? AND json_extract(data, '$." field "') = ?")
             type value]
            {:builder-fn rs/as-unqualified-kebab-maps})
          row->doc))

(defn- find-all-by-owner-or-public [type owner]
  (if (nil? owner)
    (find-docs type)
    (->> (jdbc/execute! (conn)
           ["SELECT id, type, data FROM documents WHERE type = ? AND (json_extract(data, '$.owner') = ? OR json_extract(data, '$.public') = 'true')"
            type owner]
           {:builder-fn rs/as-unqualified-kebab-maps})
         (mapv row->doc))))

;; --- Migration ---

(defn migrations []
  (->> (find-docs "migration")
       (map :name)
       (map keyword)
       (set)))

(defn record-migration [name result]
  (create-doc {:type "migration" :name name :result result}))

;; --- Secret ---

(defn create-secret [secret]
  (create-doc (assoc secret :type "secret")))

(defn get-secret []
  (find-doc {} "secret"))

;; --- Registry: Dockerhub ---

(defn dockerhubs [owner]
  (find-all-by-owner-or-public "dockerhub" owner))

(defn dockerhub
  ([id] (get-doc id))
  ([username owner]
   (some-> (jdbc/execute-one! (conn)
             ["SELECT id, type, data FROM documents WHERE type = 'dockerhub' AND json_extract(data, '$.username') = ? AND json_extract(data, '$.owner') = ?"
              username owner]
             {:builder-fn rs/as-unqualified-kebab-maps})
           row->doc)))

(defn create-dockerhub [docker-user]
  (create-doc (assoc docker-user :type "dockerhub")))

(defn update-dockerhub [docker-user delta]
  (update-doc docker-user (dissoc delta :_id :_rev :username)))

(defn delete-dockerhub [docker-user]
  (delete-doc docker-user))

;; --- Registry: v2 ---

(defn registries-v2 [owner]
  (find-all-by-owner-or-public "v2" owner))

(defn registry-v2
  ([id] (get-doc id))
  ([name owner]
   (some-> (jdbc/execute-one! (conn)
             ["SELECT id, type, data FROM documents WHERE type = 'v2' AND json_extract(data, '$.name') = ? AND json_extract(data, '$.owner') = ?"
              name owner]
             {:builder-fn rs/as-unqualified-kebab-maps})
           row->doc)))

(defn create-v2-registry [registry]
  (create-doc (assoc registry :type "v2")))

(defn update-v2-registry [registry delta]
  (update-doc registry (dissoc delta :_id :_rev :name)))

(defn delete-v2-registry [registry]
  (delete-doc registry))

;; --- Registry: ECR ---

(defn registries-ecr [owner]
  (find-all-by-owner-or-public "ecr" owner))

(defn registry-ecr
  ([id] (get-doc id))
  ([user owner]
   (some-> (jdbc/execute-one! (conn)
             ["SELECT id, type, data FROM documents WHERE type = 'ecr' AND json_extract(data, '$.user') = ? AND json_extract(data, '$.owner') = ?"
              user owner]
             {:builder-fn rs/as-unqualified-kebab-maps})
           row->doc)))

(defn create-ecr-registry [ecr]
  (create-doc (assoc ecr :type "ecr")))

(defn update-ecr-registry [ecr delta]
  (update-doc ecr (dissoc delta :_id :_rev)))

(defn delete-ecr-registry [ecr]
  (delete-doc ecr))

;; --- Registry: ACR ---

(defn registries-acr [owner]
  (find-all-by-owner-or-public "acr" owner))

(defn registry-acr
  ([id] (get-doc id))
  ([user owner]
   (some-> (jdbc/execute-one! (conn)
             ["SELECT id, type, data FROM documents WHERE type = 'acr' AND json_extract(data, '$.user') = ? AND json_extract(data, '$.owner') = ?"
              user owner]
             {:builder-fn rs/as-unqualified-kebab-maps})
           row->doc)))

(defn create-acr-registry [acr]
  (create-doc (assoc acr :type "acr")))

(defn update-acr-registry [acr delta]
  (update-doc acr (dissoc delta :_id :_rev)))

(defn delete-acr-registry [acr]
  (delete-doc acr))

;; --- Registry: GitLab ---

(defn registries-gitlab [owner]
  (find-all-by-owner-or-public "gitlab" owner))

(defn registry-gitlab
  ([id] (get-doc id))
  ([username owner]
   (some-> (jdbc/execute-one! (conn)
             ["SELECT id, type, data FROM documents WHERE type = 'gitlab' AND json_extract(data, '$.username') = ? AND json_extract(data, '$.owner') = ?"
              username owner]
             {:builder-fn rs/as-unqualified-kebab-maps})
           row->doc)))

(defn create-gitlab-registry [registry]
  (create-doc (assoc registry :type "gitlab")))

(defn update-gitlab-registry [registry delta]
  (update-doc registry (dissoc delta :_id :_rev :name)))

(defn delete-gitlab-registry [registry]
  (delete-doc registry))

;; --- User ---

(defn users []
  (find-docs "user"))

(defn user [id]
  (get-doc id))

(defn user-registries [username]
  (->> (jdbc/execute! (conn)
         ["SELECT id, type, data FROM documents WHERE type IN ('dockerhub','v2','ecr','acr','gitlab') AND json_extract(data, '$.owner') = ?"
          username]
         {:builder-fn rs/as-unqualified-kebab-maps})
       (mapv row->doc)))

(defn user-by-username [username]
  (find-by-field "user" "username" username))

(defn create-user [user]
  (create-doc (assoc user :type "user")))

(defn delete-user [user]
  (delete-doc user))

(defn delete-user-registries [username]
  (jdbc/execute-one! (conn)
    ["DELETE FROM documents WHERE type IN ('dockerhub','v2','ecr','acr','gitlab') AND json_extract(data, '$.owner') = ?"
     username]))

(defn update-user [user delta]
  (update-doc user (select-keys delta [:role :email])))

(defn change-password [user encrypted-password]
  (update-doc user :password encrypted-password))

(defn set-api-token [user api-token]
  (update-doc user :api-token api-token))

(defn update-dashboard [user dashboard-type dashboard]
  (update-doc user dashboard-type dashboard))

;; --- Stackfile ---

(defn stackfiles []
  (find-docs "stackfile"))

(defn stackfile [name]
  (find-by-field "stackfile" "name" name))

(defn create-stackfile [sf]
  (create-doc (assoc sf :type "stackfile")))

(defn update-stackfile [sf delta]
  (update-doc sf (select-keys delta [:spec :previousSpec])))

(defn delete-stackfile [sf]
  (delete-doc sf))
