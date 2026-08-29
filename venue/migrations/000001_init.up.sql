CREATE TYPE kitchen_order_state AS ENUM (
    'NEW',
    'ACCEPTED',
    'COOKING',
    'READY',
    'HANDED_OVER',
    'REJECTED',
    'CANCELLED'
);

CREATE TABLE dishes (
    sku                  text PRIMARY KEY,
    name                 text NOT NULL,
    description          text NOT NULL DEFAULT '',
    price                bigint NOT NULL CHECK (price >= 0),
    stock                integer CHECK (stock >= 0),
    is_available         boolean NOT NULL DEFAULT true,
    category_external_id text NOT NULL,
    category_name        text NOT NULL,
    category_position    integer NOT NULL DEFAULT 0,
    position             integer NOT NULL DEFAULT 0,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX dishes_menu_order_idx ON dishes (category_position, position, sku);

CREATE TABLE incoming_orders (
    main_order_id    uuid PRIMARY KEY,
    payload          jsonb NOT NULL,
    state            kitchen_order_state NOT NULL DEFAULT 'NEW',
    received_at      timestamptz NOT NULL DEFAULT now(),
    state_changed_at timestamptz NOT NULL DEFAULT now(),
    decided_at       timestamptz
);

CREATE INDEX incoming_orders_state_changed_at_idx ON incoming_orders (state, state_changed_at);

CREATE TABLE processed_events (
    consumer     text NOT NULL,
    event_id     uuid NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
