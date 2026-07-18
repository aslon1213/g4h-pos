-- The app originates from MongoDB, where optional references were stored as
-- empty strings ("") rather than NULL. In PostgreSQL an empty-string value in a
-- foreign-key column fails the constraint (there is no row with id = '').
--
-- Rather than rewrite every repository to emit NULL for "no relation", we drop
-- the FK constraints on the genuinely-optional reference columns (a product may
-- have no brand, an income line no supplier, a category no parent). The columns
-- and their indexes remain for joins; only referential enforcement is relaxed —
-- matching the source data model. The always-set, owned child-table cascades
-- (cart_items, order_items, product_stock, bnpl_products, addresses, …) keep
-- their foreign keys.

-- +goose Up
ALTER TABLE products               DROP CONSTRAINT IF EXISTS products_brand_id_fkey;
ALTER TABLE product_income_history DROP CONSTRAINT IF EXISTS product_income_history_supplier_id_fkey;
ALTER TABLE categories             DROP CONSTRAINT IF EXISTS categories_parent_id_fkey;

-- +goose Down
ALTER TABLE products               ADD CONSTRAINT products_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES brands (id) ON DELETE SET NULL;
ALTER TABLE product_income_history ADD CONSTRAINT product_income_history_supplier_id_fkey FOREIGN KEY (supplier_id) REFERENCES suppliers (id) ON DELETE SET NULL;
ALTER TABLE categories             ADD CONSTRAINT categories_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES categories (id) ON DELETE SET NULL;
