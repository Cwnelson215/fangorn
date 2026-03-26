CREATE TABLE IF NOT EXISTS bank_csv_formats (
    id SERIAL PRIMARY KEY,
    bank_name TEXT NOT NULL UNIQUE,
    date_column TEXT NOT NULL,
    amount_column TEXT NOT NULL,
    description_column TEXT NOT NULL,
    category_column TEXT,
    negate_amounts BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
