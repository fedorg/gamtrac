-- delete from file_history;
-- delete from files;
-- delete from revisions;
-- alter sequence file_history_file_history_id_seq restart;
-- alter sequence files_file_id_seq restart;
-- alter sequence revisions_revision_id_seq restart;

-- alter table file_history add constraint file_history_filename_revision_id_unique unique (filename, revision_id);


drop table file_history;
create table file_history (
    file_history_id BIGSERIAL PRIMARY KEY,
    action text NOT NULL CHECK (action IN ('I','D','U','E')),
    action_tstamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    filename TEXT NOT NULL,
    revision_id int,
    "data" jsonb
    -- like files
);


-- ASSUMPTION: filename is globally unique and we don't know about file_id
-- ASSUMPTION: no renames
-- ASSUMPTION: no DELETEs


-- This does not work with 'track this' option on hasura console.

create or replace function trigger_on_files_changed() returns trigger
as $$
begin
    -- IF (TG_OP = 'DELETE') THEN
    --     RAISE EXCEPTION 'Can only delete rows by setting filename = NULL';
    IF (TG_OP = 'INSERT') THEN
        INSERT INTO file_history (action, filename, revision_id, "data")
         VALUES ('I', NEW.filename, NEW.revision_id, NEW.data);
    ELSIF (TG_OP = 'UPDATE') THEN
        IF NEW.filename is NULL THEN  -- delete records by setting filename to NULL
            INSERT INTO file_history (action, filename, revision_id, "data")
                VALUES ('D', OLD.filename, NEW.revision_id, OLD.data);
            DELETE FROM files where (file_id = NEW.file_id);
            RETURN NEW; -- might cause problems
        ELSIF NEW.filename <> OLD.filename THEN
            RAISE EXCEPTION 'Cannot change filename, you should delete the file and create a new one with a different name.';
        ELSIF NEW.file_id <> OLD.file_id THEN
            RAISE EXCEPTION 'Cannot change file_id, you should delete the file and create a new one with a different name.';
        END IF;
        IF (to_jsonb(OLD.data) - 'QueuedAt' - 'ProcessedAt' - 'Errors') <> (to_jsonb(NEW.data) - 'QueuedAt' - 'ProcessedAt' - 'Errors') THEN
            INSERT INTO file_history (action, filename, revision_id, "data")
             VALUES ('U', OLD.filename, NEW.revision_id, OLD.data);
        ELSIF NEW.revision_id <> OLD.revision_id THEN
            RETURN NULL;
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