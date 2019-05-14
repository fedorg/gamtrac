drop table file_history;
create table file_history (
    file_history_id BIGSERIAL PRIMARY KEY,
    action text NOT NULL CHECK (action IN ('I','D','U','E')),
    action_tstamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    filename TEXT NOT NULL,
    revision_id int,
    data_diff jsonb
    -- like files
);


-- ASSUMPTION: filename is globally unique and we don't know about file_id
-- ASSUMPTION: no renames
-- ASSUMPTION: no DELETEs

CREATE OR REPLACE FUNCTION json_diff(l JSONB, r JSONB) RETURNS JSONB AS
$json_diff$
    SELECT jsonb_object_agg(a.key, a.value) FROM
        ( SELECT key, value FROM jsonb_each(l) ) a LEFT OUTER JOIN
        ( SELECT key, value FROM jsonb_each(r) ) b ON a.key = b.key
    WHERE a.value != b.value OR b.key IS NULL;
$json_diff$
    LANGUAGE sql;

create or replace function trigger_on_files_changed() returns trigger
as $$
begin
    -- IF (TG_OP = 'DELETE') THEN
    --     RAISE EXCEPTION 'Can only delete rows by setting filename = NULL';
    IF (TG_OP = 'INSERT') THEN
        INSERT INTO file_history (action, filename, revision_id, data_diff)
         VALUES ('I', NEW.filename, NEW.revision_id, NEW.data);
    ELSIF (TG_OP = 'UPDATE') THEN
        IF NEW.filename is NULL THEN  -- delete records by setting filename to NULL
            INSERT INTO file_history (action, filename, revision_id, data_diff)
                VALUES ('D', OLD.filename, NEW.revision_id, OLD.data);
            DELETE FROM files where (file_id = NEW.file_id);
            RETURN NEW; -- might cause problems
        ELSIF NEW.filename <> OLD.filename THEN
            RAISE EXCEPTION 'Cannot change filename, you should delete the file and create a new one with a different name.';
        ELSIF NEW.file_id <> OLD.file_id THEN
            RAISE EXCEPTION 'Cannot change file_id, you should delete the file and create a new one with a different name.';
        END IF;
        IF (to_jsonb(OLD.data) - 'QueuedAt' - 'ProcessedAt' - 'Errors') <> (to_jsonb(NEW.data) - 'QueuedAt' - 'ProcessedAt' - 'Errors') THEN
            INSERT INTO file_history (action, filename, revision_id, data_diff)
             VALUES ('U', OLD.filename, NEW.revision_id, jsonb_minus(NEW.data, OLD.data));  -- doesn't work on deletets
        END IF;
    END IF;
    RETURN NEW;
end;
$$ LANGUAGE 'plpgsql';


DROP TRIGGER IF EXISTS trigger_files_changed ON files;
create trigger trigger_files_changed
AFTER UPDATE OR INSERT on files -- ignores return value
for each row
execute procedure trigger_on_files_changed();