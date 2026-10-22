INSERT INTO shop (id, name, description, opened, updated, delay, version, staff, rating, margin, revenue, revenue_currency, area, area_unit, open, condition, logo, terms, widget, address_id, manager_id) VALUES ('lyon', 'Lyon, it''s "Croix-Rousse"', 'A shop on the hill,
two lines in its first paragraph.

Its second paragraph.', '2024-03-01', '2026-10-22 14:30:05 +02:00', '1 h 30 min', '1.2.3', 12, 4.5, 12.5, 1250000.5, 'EUR', 48, 'm2', 1, 'good', X'3c73766720786d6c6e733d22687474703a2f2f7777772e77332e6f72672f323030302f737667222077696474683d223122206865696768743d2231222f3e0a', X'4e6f2072657475726e732e0a', '<b>open</b>', 'lyon-address', 'ana');
INSERT INTO shop_suppliers (shop_id, supplier_id) VALUES ('lyon', 'acme');
INSERT INTO shop_suppliers (shop_id, supplier_id) VALUES ('lyon', 'bolt');
INSERT INTO shop_tags (shop_id, value) VALUES ('lyon', 'organic');
INSERT INTO shop_tags (shop_id, value) VALUES ('lyon', 'bulk');
INSERT INTO shop_photos (shop_id, value) VALUES ('lyon', X'3c73766720786d6c6e733d22687474703a2f2f7777772e77332e6f72672f323030302f737667222f3e0a');
INSERT INTO shop_photos (shop_id, value) VALUES ('lyon', X'3c7376672f3e0a');
INSERT INTO address (id, street, city) VALUES ('lyon-address', '12 rue d''Austerlitz', 'Lyon');
INSERT INTO person (id, shop_employees_id, name) VALUES ('ana', 'lyon', 'Ana');
INSERT INTO person_friends (person_id, friends_id) VALUES ('ana', 'bo');
INSERT INTO person_friends (person_id, friends_id) VALUES ('ana', 'cy');
INSERT INTO person (id, shop_employees_id, name) VALUES ('bo', 'lyon', 'Bo');
INSERT INTO person (id, shop_owners_id, name) VALUES ('cy', 'lyon', 'Cy');
INSERT INTO "order" (shop_id, id, date, customer_id) VALUES ('lyon', 'o1', '2026-10-01', 'cy');
INSERT INTO line (shop_id, order_id, id, product_id, "index", quantity, price, price_currency) VALUES ('lyon', 'o1', 'l1', 'soap', 1, 2, 3.5, 'EUR');
INSERT INTO line_notes (shop_id, order_id, line_id, value) VALUES ('lyon', 'o1', 'l1', 'fragile');
INSERT INTO line_notes (shop_id, order_id, line_id, value) VALUES ('lyon', 'o1', 'l1', 'gift');
INSERT INTO line (shop_id, order_id, id, product_id, quantity, price, price_currency) VALUES ('lyon', 'o1', 'l2', 'brush', 1, 12, 'EUR');
INSERT INTO shop (id, name, description, opened, staff, margin, revenue, revenue_currency, open, condition, address_id, manager_id) VALUES ('paris', 'Paris', 'The second shop.', '2025-01-15', 3, 8, 90000, 'EUR', 0, 'new', 'paris-address', 'zed');
INSERT INTO address (id, street, city) VALUES ('paris-address', '1 rue de Rivoli', 'Paris');
INSERT INTO person (id, shop_employees_id, name) VALUES ('dee', 'paris', 'Dee');
INSERT INTO "order" (shop_id, id, date) VALUES ('paris', 'o1', '2026-10-02');
INSERT INTO line (shop_id, order_id, id, product_id, quantity, price, price_currency) VALUES ('paris', 'o1', 'l1', 'soap', 10, 3.2, 'EUR');
INSERT INTO supplier (id, name) VALUES ('acme', 'Acme');
INSERT INTO product (id, supplier_products_id, name, supplier_id) VALUES ('soap', 'acme', 'Soap', 'acme');
INSERT INTO product_prices (product_id, value, value_currency) VALUES ('soap', 3.5, 'EUR');
INSERT INTO product_prices (product_id, value, value_currency) VALUES ('soap', 3.2, 'EUR');
INSERT INTO product_sizes (product_id, value, value_unit) VALUES ('soap', 250, 'ml');
INSERT INTO product_sizes (product_id, value) VALUES ('soap', 2);
INSERT INTO product_conditions (product_id, value) VALUES ('soap', 'new');
INSERT INTO product_conditions (product_id, value) VALUES ('soap', 'worn');
INSERT INTO product (id, supplier_products_id, name) VALUES ('brush', 'acme', 'Brush');
INSERT INTO supplier (id, name) VALUES ('bolt', 'Bolt');
INSERT INTO person (id, name) VALUES ('zed', 'Zed');
INSERT INTO person_friends (person_id, friends_id) VALUES ('zed', 'zed');
