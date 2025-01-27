CREATE TABLE IF NOT EXISTS public."users"
(
    "id"            varchar   NOT NULL,
    "name"          varchar   NOT NULL,
    "email"         varchar   NOT NULL UNIQUE,
    "password_hash" varchar   NOT NULL,
    "created_at"    timestamp NOT NULL,
    "updated_at"    timestamp NOT NULL,
    PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS public."articles"
(
    "id"                   varchar   NOT NULL,
    "title"                varchar   NOT NULL,
    "body"                 text      NOT NULL,
    "user_id"              varchar   NOT NULL,
    "total_favorite_count" bigint    NOT NULL,
    "created_at"           timestamp NOT NULL,
    "updated_at"           timestamp NOT NULL,
    PRIMARY KEY ("id")
-- DSQLは外部キーをサポートしていない
--    FOREIGN KEY ("user_id") REFERENCES "users" ("id")
);

CREATE TABLE IF NOT EXISTS public."users_articles"
(
    "user_id"    varchar   NOT NULL,
    "article_id" varchar   NOT NULL,
    "created_at" timestamp NOT NULL,
    "updated_at" timestamp NOT NULL,
    PRIMARY KEY ("user_id", "article_id")
-- DSQLは外部キーをサポートしていない
--    FOREIGN KEY ("article_id") REFERENCES "articles" ("id"),
--    FOREIGN KEY ("user_id") REFERENCES "users" ("id")
);
