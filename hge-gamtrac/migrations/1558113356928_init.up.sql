SET xmloption = content;
CREATE FUNCTION public.json_diff(l jsonb, r jsonb) RETURNS jsonb
    LANGUAGE sql
    AS $$
    SELECT jsonb_object_agg(a.key, a.value) FROM
        ( SELECT key, value FROM jsonb_each(l) ) a FULL OUTER JOIN
        ( SELECT key, value FROM jsonb_each(r) ) b ON a.key = b.key
    WHERE a.value != b.value OR b.key IS NULL;
$$;
CREATE FUNCTION public.jsonb_minus(arg1 jsonb, arg2 jsonb) RETURNS jsonb
    LANGUAGE sql
    AS $$
SELECT 
	COALESCE(json_object_agg(key, value), '{}')::jsonb
FROM 
	jsonb_each(arg1)
WHERE 
	arg1 -> key <> arg2 -> key 
	OR arg2 -> key IS NULL
$$;
CREATE FUNCTION public.trigger_on_files_changed() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;
CREATE TABLE public.domain_users (
    sid text NOT NULL,
    username text NOT NULL,
    name text NOT NULL,
    groups jsonb DEFAULT '[]'::jsonb NOT NULL
);
CREATE TABLE public.file_history (
    file_history_id bigint NOT NULL,
    action text NOT NULL,
    action_tstamp timestamp with time zone DEFAULT now() NOT NULL,
    filename text NOT NULL,
    revision_id integer,
    data jsonb,
    CONSTRAINT file_history_action_check CHECK ((action = ANY (ARRAY['I'::text, 'D'::text, 'U'::text, 'E'::text])))
);
CREATE SEQUENCE public.file_history_file_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.file_history_file_history_id_seq OWNED BY public.file_history.file_history_id;
CREATE TABLE public.files (
    file_id bigint NOT NULL,
    revision_id integer NOT NULL,
    filename text,
    data jsonb NOT NULL
);
CREATE SEQUENCE public.files_file_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.files_file_id_seq OWNED BY public.files.file_id;
CREATE TABLE public.revisions (
    revision_id integer NOT NULL,
    started timestamp with time zone DEFAULT now() NOT NULL,
    completed timestamp with time zone
);
CREATE SEQUENCE public.revisions_revision_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.revisions_revision_id_seq OWNED BY public.revisions.revision_id;
ALTER TABLE ONLY public.file_history ALTER COLUMN file_history_id SET DEFAULT nextval('public.file_history_file_history_id_seq'::regclass);
ALTER TABLE ONLY public.files ALTER COLUMN file_id SET DEFAULT nextval('public.files_file_id_seq'::regclass);
ALTER TABLE ONLY public.revisions ALTER COLUMN revision_id SET DEFAULT nextval('public.revisions_revision_id_seq'::regclass);
ALTER TABLE ONLY public.domain_users
    ADD CONSTRAINT "domainUsers_pkey" PRIMARY KEY (sid);
ALTER TABLE ONLY public.domain_users
    ADD CONSTRAINT "domainUsers_username_key" UNIQUE (username);
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_filename_revision_id_unique UNIQUE (filename, revision_id);
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_pkey PRIMARY KEY (file_history_id);
ALTER TABLE ONLY public.files
    ADD CONSTRAINT files_file_id_key UNIQUE (file_id);
ALTER TABLE ONLY public.files
    ADD CONSTRAINT files_filename_key UNIQUE (filename);
ALTER TABLE ONLY public.files
    ADD CONSTRAINT files_pkey PRIMARY KEY (file_id);
ALTER TABLE ONLY public.revisions
    ADD CONSTRAINT revisions_pkey PRIMARY KEY (revision_id);
CREATE TRIGGER trigger_files_changed AFTER INSERT OR UPDATE ON public.files FOR EACH ROW EXECUTE PROCEDURE public.trigger_on_files_changed();
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES public.revisions(revision_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE ONLY public.files
    ADD CONSTRAINT files_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES public.revisions(revision_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
