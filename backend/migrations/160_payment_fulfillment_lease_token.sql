-- Opaque ownership token for fencing payment fulfillment workers across lease recovery.
ALTER TABLE payment_orders
ADD COLUMN IF NOT EXISTS fulfillment_lease_token VARCHAR(64);
