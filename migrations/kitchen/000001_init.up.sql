CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TYPE order_status AS ENUM (
    'CREATED',
    'ACCEPTED',
    'COOKING',
    'READY',
    'DELIVERING',
    'DELIVERED',
    'REJECTED',
    'CANCELLED'
);

CREATE TYPE order_actor AS ENUM ('customer', 'venue', 'system');

CREATE TABLE cuisines (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug     text NOT NULL UNIQUE,
    name     text NOT NULL,
    position integer NOT NULL DEFAULT 0
);

CREATE TABLE venues (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             text NOT NULL UNIQUE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    address          text NOT NULL,
    lat              double precision,
    lon              double precision,
    is_active        boolean NOT NULL DEFAULT true,
    is_open          boolean NOT NULL DEFAULT false,
    min_order_amount bigint NOT NULL DEFAULT 0 CHECK (min_order_amount >= 0),
    delivery_fee     bigint NOT NULL DEFAULT 0 CHECK (delivery_fee >= 0),
    avg_cook_minutes integer NOT NULL DEFAULT 20 CHECK (avg_cook_minutes > 0),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX venues_name_trgm_idx ON venues USING gin (name gin_trgm_ops);

CREATE TABLE venue_cuisines (
    venue_id   uuid NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    cuisine_id uuid NOT NULL REFERENCES cuisines (id) ON DELETE RESTRICT,
    PRIMARY KEY (venue_id, cuisine_id)
);

CREATE INDEX venue_cuisines_cuisine_id_venue_id_idx ON venue_cuisines (cuisine_id, venue_id);

CREATE TABLE venue_api_keys (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id   uuid NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    key_hash   bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE TABLE menu_categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id    uuid NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    external_id text NOT NULL,
    name        text NOT NULL,
    position    integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (venue_id, external_id)
);

CREATE TABLE menu_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id     uuid NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    category_id  uuid NOT NULL REFERENCES menu_categories (id) ON DELETE CASCADE,
    external_id  text NOT NULL,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    price        bigint NOT NULL CHECK (price >= 0),
    is_available boolean NOT NULL DEFAULT true,
    stock_qty    integer CHECK (stock_qty >= 0),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (venue_id, external_id)
);

CREATE INDEX menu_items_category_id_position_idx ON menu_items (category_id, name);

CREATE TABLE carts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL UNIQUE,
    venue_id   uuid REFERENCES venues (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- price_snapshot is the price the item was added at: cart validation reports
-- a price change by comparing it with the current one.
CREATE TABLE cart_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id        uuid NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    menu_item_id   uuid NOT NULL REFERENCES menu_items (id) ON DELETE CASCADE,
    qty            integer NOT NULL CHECK (qty > 0),
    price_snapshot bigint NOT NULL CHECK (price_snapshot >= 0),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cart_id, menu_item_id)
);

CREATE SEQUENCE order_number_seq AS bigint START WITH 100000;

CREATE TABLE orders (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number           text NOT NULL UNIQUE DEFAULT 'AK-' || nextval('order_number_seq'),
    user_id          uuid NOT NULL,
    venue_id         uuid NOT NULL REFERENCES venues (id) ON DELETE RESTRICT,
    status           order_status NOT NULL DEFAULT 'CREATED',
    items_total      bigint NOT NULL CHECK (items_total >= 0),
    delivery_fee     bigint NOT NULL CHECK (delivery_fee >= 0),
    total            bigint NOT NULL CHECK (total >= 0),
    address          text,
    phone            text NOT NULL,
    comment          text NOT NULL DEFAULT '',
    eta_minutes      integer CHECK (eta_minutes > 0),
    rejection_reason text,
    version          bigint NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX orders_user_id_created_at_idx ON orders (user_id, created_at DESC);
CREATE INDEX orders_venue_id_status_idx ON orders (venue_id, status);

CREATE TABLE order_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    menu_item_id   uuid REFERENCES menu_items (id) ON DELETE SET NULL,
    external_id    text NOT NULL,
    name_snapshot  text NOT NULL,
    price_snapshot bigint NOT NULL CHECK (price_snapshot >= 0),
    qty            integer NOT NULL CHECK (qty > 0),
    line_total     bigint GENERATED ALWAYS AS (qty * price_snapshot) STORED
);

CREATE INDEX order_items_order_id_idx ON order_items (order_id);

CREATE TABLE order_status_history (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status order_status,
    to_status   order_status NOT NULL,
    actor       order_actor NOT NULL,
    reason      text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX order_status_history_order_id_id_idx ON order_status_history (order_id, id);

CREATE TABLE idempotency_keys (
    user_id         uuid NOT NULL,
    key             text NOT NULL,
    endpoint        text NOT NULL,
    request_hash    bytea NOT NULL,
    response_status integer,
    response_body   jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    PRIMARY KEY (user_id, key)
);

CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);

CREATE TABLE outbox_messages (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id          uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    topic             text NOT NULL,
    key               text NOT NULL,
    event_type        text NOT NULL,
    aggregate_id      uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    payload           jsonb NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    published_at      timestamptz,
    attempts          integer NOT NULL DEFAULT 0,
    last_error        text
);

CREATE INDEX outbox_messages_unpublished_idx ON outbox_messages (created_at) WHERE published_at IS NULL;

CREATE TABLE processed_events (
    consumer     text NOT NULL,
    event_id     uuid NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
