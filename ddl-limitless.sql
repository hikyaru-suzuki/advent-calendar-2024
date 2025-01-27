CREATE TABLE IF NOT EXISTS public."users"
(
    "id"            varchar   NOT NULL,
    "name"          varchar   NOT NULL,
    "email"         varchar   NOT NULL,
    "password_hash" varchar   NOT NULL,
    "created_at"    timestamp NOT NULL,
    "updated_at"    timestamp NOT NULL,
    PRIMARY KEY ("id"),
    UNIQUE ("id", "email")
-- ERROR:  unique constraint on sharded table must include all sharded key columns
-- HINT:  UNIQUE constraint on table "public.users" lacks column "id" which is part of the shard key
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.users', ARRAY['id']);

CREATE TABLE IF NOT EXISTS public."articles"
(
    "id"                   varchar   NOT NULL,
    "title"                varchar   NOT NULL,
    "body"                 text      NOT NULL,
    "user_id"              varchar   NOT NULL,
    "total_favorite_count" bigint    NOT NULL,
    "created_at"           timestamp NOT NULL,
    "updated_at"           timestamp NOT NULL,
    PRIMARY KEY ("id"),
    FOREIGN KEY ("user_id") REFERENCES "users" ("id")
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.articles', ARRAY['id']);

CREATE TABLE IF NOT EXISTS public."users_articles"
(
    "user_id"    varchar   NOT NULL,
    "article_id" varchar   NOT NULL,
    "created_at" timestamp NOT NULL,
    "updated_at" timestamp NOT NULL,
    PRIMARY KEY ("user_id", "article_id"),
    FOREIGN KEY ("article_id") REFERENCES "articles" ("id"),
    FOREIGN KEY ("user_id") REFERENCES "users" ("id")
);
CALL rds_aurora.limitless_alter_table_type_sharded('public.users_articles', ARRAY['user_id', 'article_id']);
