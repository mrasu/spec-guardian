CREATE TABLE IF NOT EXISTS orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  request_id VARCHAR(128) NOT NULL,
  payload TEXT NOT NULL,
  delivery_id CHAR(36) NULL,
  delivery_data TEXT NULL,
  status ENUM('CREATED', 'DELIVERING', 'DELIVERED') NOT NULL DEFAULT 'CREATED',
  PRIMARY KEY (id),
  UNIQUE KEY orders_request_id (request_id),
  UNIQUE KEY orders_delivery_id (delivery_id)
);
