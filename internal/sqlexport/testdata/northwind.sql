CREATE TABLE category (
  id            TEXT NOT NULL,
  category_name TEXT,
  description   TEXT,
  picture       BLOB,
  PRIMARY KEY (id)
);

CREATE TABLE customer (
  id            TEXT NOT NULL,
  company_name  TEXT,
  contact_name  TEXT,
  contact_title TEXT,
  address       TEXT,
  city          TEXT,
  region        TEXT,
  postal_code   TEXT,
  country       TEXT,
  phone         TEXT,
  fax           TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE customer_customer_demographics (
  customer_id             TEXT NOT NULL REFERENCES customer (id) DEFERRABLE INITIALLY DEFERRED,
  customer_demographic_id TEXT NOT NULL REFERENCES customer_demographic (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (customer_id, customer_demographic_id)
);

CREATE TABLE customer_demographic (
  id            TEXT NOT NULL,
  customer_desc TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE employee (
  id                TEXT NOT NULL,
  last_name         TEXT,
  first_name        TEXT,
  title             TEXT,
  title_of_courtesy TEXT,
  birth_date        DATE,
  hire_date         DATE,
  address           TEXT,
  city              TEXT,
  region            TEXT,
  postal_code       TEXT,
  country           TEXT,
  home_phone        TEXT,
  extension         TEXT,
  photo             BLOB,
  notes             TEXT,
  reports_to_id     TEXT REFERENCES employee (id) DEFERRABLE INITIALLY DEFERRED,
  photo_path        TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE employee_territories (
  employee_id  TEXT NOT NULL REFERENCES employee (id) DEFERRABLE INITIALLY DEFERRED,
  territory_id TEXT NOT NULL REFERENCES territory (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (employee_id, territory_id)
);

CREATE TABLE "order" (
  id               TEXT NOT NULL,
  customer_id      TEXT REFERENCES customer (id) DEFERRABLE INITIALLY DEFERRED,
  employee_id      TEXT REFERENCES employee (id) DEFERRABLE INITIALLY DEFERRED,
  order_date       DATE,
  required_date    DATE,
  shipped_date     DATE,
  ship_via_id      TEXT REFERENCES shipper (id) DEFERRABLE INITIALLY DEFERRED,
  freight          NUMERIC,
  ship_name        TEXT,
  ship_address     TEXT,
  ship_city        TEXT,
  ship_region      TEXT,
  ship_postal_code TEXT,
  ship_country     TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE order_detail (
  order_id   TEXT NOT NULL REFERENCES "order" (id) DEFERRABLE INITIALLY DEFERRED,
  id         TEXT NOT NULL,
  product_id TEXT NOT NULL REFERENCES product (id) DEFERRABLE INITIALLY DEFERRED,
  unit_price NUMERIC NOT NULL,
  quantity   INTEGER NOT NULL,
  discount   NUMERIC NOT NULL,
  PRIMARY KEY (order_id, id)
);

CREATE TABLE product (
  id                TEXT NOT NULL,
  product_name      TEXT NOT NULL,
  supplier_id       TEXT REFERENCES supplier (id) DEFERRABLE INITIALLY DEFERRED,
  category_id       TEXT REFERENCES category (id) DEFERRABLE INITIALLY DEFERRED,
  quantity_per_unit TEXT,
  unit_price        NUMERIC,
  units_in_stock    INTEGER,
  units_on_order    INTEGER,
  reorder_level     INTEGER,
  discontinued      TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE region (
  id                 TEXT NOT NULL,
  region_description TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE shipper (
  id           TEXT NOT NULL,
  company_name TEXT NOT NULL,
  phone        TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE supplier (
  id            TEXT NOT NULL,
  company_name  TEXT NOT NULL,
  contact_name  TEXT,
  contact_title TEXT,
  address       TEXT,
  city          TEXT,
  region        TEXT,
  postal_code   TEXT,
  country       TEXT,
  phone         TEXT,
  fax           TEXT,
  home_page     TEXT,
  PRIMARY KEY (id)
);

CREATE TABLE territory (
  id                    TEXT NOT NULL,
  territory_description TEXT NOT NULL,
  region_id             TEXT NOT NULL REFERENCES region (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);
