CREATE TABLE shop (
  id               TEXT NOT NULL,
  name             TEXT NOT NULL,
  description      TEXT NOT NULL,
  opened           DATE NOT NULL,
  updated          DATETIME,
  delay            TEXT,
  version          TEXT,
  staff            INTEGER NOT NULL,
  rating           NUMERIC,
  margin           NUMERIC NOT NULL,
  revenue          NUMERIC NOT NULL,
  revenue_currency TEXT NOT NULL,
  area             NUMERIC,
  area_unit        TEXT,
  open             INTEGER NOT NULL CHECK (open IN (0, 1)),
  condition        TEXT NOT NULL CHECK (condition IN ('new', 'good', 'worn')),
  logo             BLOB,
  terms            BLOB,
  widget           TEXT,
  address_id       TEXT NOT NULL REFERENCES address (id) DEFERRABLE INITIALLY DEFERRED,
  manager_id       TEXT NOT NULL REFERENCES person (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);

CREATE TABLE shop_suppliers (
  shop_id     TEXT NOT NULL REFERENCES shop (id) DEFERRABLE INITIALLY DEFERRED,
  supplier_id TEXT NOT NULL REFERENCES supplier (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (shop_id, supplier_id)
);

CREATE TABLE shop_tags (
  shop_id TEXT NOT NULL REFERENCES shop (id) DEFERRABLE INITIALLY DEFERRED,
  value   TEXT NOT NULL
);

CREATE TABLE shop_photos (
  shop_id TEXT NOT NULL REFERENCES shop (id) DEFERRABLE INITIALLY DEFERRED,
  value   BLOB NOT NULL
);

CREATE TABLE address (
  id     TEXT NOT NULL,
  street TEXT NOT NULL,
  city   TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE person (
  id                TEXT NOT NULL,
  shop_employees_id TEXT REFERENCES shop (id) DEFERRABLE INITIALLY DEFERRED,
  shop_owners_id    TEXT REFERENCES shop (id) DEFERRABLE INITIALLY DEFERRED,
  name              TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE person_friends (
  person_id  TEXT NOT NULL REFERENCES person (id) DEFERRABLE INITIALLY DEFERRED,
  friends_id TEXT NOT NULL REFERENCES person (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (person_id, friends_id)
);

CREATE TABLE "order" (
  shop_id     TEXT NOT NULL REFERENCES shop (id) DEFERRABLE INITIALLY DEFERRED,
  id          TEXT NOT NULL,
  date        DATE NOT NULL,
  customer_id TEXT REFERENCES person (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (shop_id, id)
);

CREATE TABLE line (
  shop_id        TEXT NOT NULL,
  order_id       TEXT NOT NULL,
  id             TEXT NOT NULL,
  product_id     TEXT NOT NULL REFERENCES product (id) DEFERRABLE INITIALLY DEFERRED,
  "index"        INTEGER,
  quantity       INTEGER NOT NULL,
  price          NUMERIC NOT NULL,
  price_currency TEXT NOT NULL,
  PRIMARY KEY (shop_id, order_id, id),
  FOREIGN KEY (shop_id, order_id) REFERENCES "order" (shop_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE line_notes (
  shop_id  TEXT NOT NULL,
  order_id TEXT NOT NULL,
  line_id  TEXT NOT NULL,
  value    TEXT NOT NULL,
  FOREIGN KEY (shop_id, order_id, line_id) REFERENCES line (shop_id, order_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE supplier (
  id   TEXT NOT NULL,
  name TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE product (
  id                   TEXT NOT NULL,
  supplier_products_id TEXT REFERENCES supplier (id) DEFERRABLE INITIALLY DEFERRED,
  name                 TEXT NOT NULL,
  supplier_id          TEXT REFERENCES supplier (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);

CREATE TABLE product_prices (
  product_id     TEXT NOT NULL REFERENCES product (id) DEFERRABLE INITIALLY DEFERRED,
  value          NUMERIC NOT NULL,
  value_currency TEXT NOT NULL
);

CREATE TABLE product_sizes (
  product_id TEXT NOT NULL REFERENCES product (id) DEFERRABLE INITIALLY DEFERRED,
  value      NUMERIC NOT NULL,
  value_unit TEXT
);

CREATE TABLE product_conditions (
  product_id TEXT NOT NULL REFERENCES product (id) DEFERRABLE INITIALLY DEFERRED,
  value      TEXT NOT NULL CHECK (value IN ('new', 'good', 'worn'))
);
