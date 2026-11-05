ALTER TABLE deposits
DROP CONSTRAINT deposits_transfer_event_id_fkey;

ALTER TABLE deposits
ADD CONSTRAINT deposits_transfer_event_id_fkey
FOREIGN KEY (transfer_event_id) REFERENCES mock_transfer_events(id);
