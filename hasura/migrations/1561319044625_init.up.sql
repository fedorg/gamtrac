SET xmloption = content;
CREATE TABLE public.domain_users (
    sid text NOT NULL,
    username text NOT NULL,
    name text NOT NULL,
    groups jsonb DEFAULT '[]'::jsonb NOT NULL
);
CREATE TABLE public.endpoints (
    endpoint_id integer NOT NULL,
    path text NOT NULL,
    principal integer,
    ignore boolean DEFAULT false NOT NULL
);
CREATE SEQUENCE public.endpoints_endpoint_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.endpoints_endpoint_id_seq OWNED BY public.endpoints.endpoint_id;
CREATE TABLE public.file_history (
    file_history_id bigint NOT NULL,
    action text NOT NULL,
    action_tstamp timestamp with time zone DEFAULT now() NOT NULL,
    filename text NOT NULL,
    scan_id integer NOT NULL,
    prev_id integer DEFAULT 0 NOT NULL,
    CONSTRAINT file_history_action_check CHECK ((action = ANY (ARRAY['I'::text, 'D'::text, 'U'::text, 'E'::text]))),
    CONSTRAINT must_have_prev_unless_new_check CHECK (((prev_id <> 0) OR (action = 'C'::text)))
);
CREATE SEQUENCE public.file_history_file_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.file_history_file_history_id_seq OWNED BY public.file_history.file_history_id;
CREATE VIEW public.files AS
 SELECT DISTINCT ON (file_history.filename) file_history.file_history_id,
    file_history.action,
    file_history.action_tstamp,
    file_history.filename,
    file_history.scan_id,
    file_history.prev_id
   FROM public.file_history
  WHERE (file_history.action <> 'D'::text)
  ORDER BY file_history.filename, file_history.file_history_id DESC;
CREATE TABLE public.rule_results (
    rule_result_id integer NOT NULL,
    file_history_id integer NOT NULL,
    data jsonb,
    rule_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE SEQUENCE public.rule_results_rule_result_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.rule_results_rule_result_id_seq OWNED BY public.rule_results.rule_result_id;
CREATE TABLE public.rules (
    rule_id integer NOT NULL,
    rule text NOT NULL,
    principal integer,
    priority integer NOT NULL,
    ignore boolean DEFAULT false NOT NULL
);
CREATE SEQUENCE public.rules_rule_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.rules_rule_id_seq OWNED BY public.rules.rule_id;
CREATE TABLE public.scans (
    scan_id integer NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone
);
CREATE SEQUENCE public.scans_scan_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.scans_scan_id_seq OWNED BY public.scans.scan_id;
ALTER TABLE ONLY public.endpoints ALTER COLUMN endpoint_id SET DEFAULT nextval('public.endpoints_endpoint_id_seq'::regclass);
ALTER TABLE ONLY public.file_history ALTER COLUMN file_history_id SET DEFAULT nextval('public.file_history_file_history_id_seq'::regclass);
ALTER TABLE ONLY public.rule_results ALTER COLUMN rule_result_id SET DEFAULT nextval('public.rule_results_rule_result_id_seq'::regclass);
ALTER TABLE ONLY public.rules ALTER COLUMN rule_id SET DEFAULT nextval('public.rules_rule_id_seq'::regclass);
ALTER TABLE ONLY public.scans ALTER COLUMN scan_id SET DEFAULT nextval('public.scans_scan_id_seq'::regclass);
ALTER TABLE ONLY public.domain_users
    ADD CONSTRAINT "domainUsers_pkey" PRIMARY KEY (sid);
ALTER TABLE ONLY public.domain_users
    ADD CONSTRAINT "domainUsers_username_key" UNIQUE (username);
ALTER TABLE ONLY public.endpoints
    ADD CONSTRAINT endpoints_path_key UNIQUE (path);
ALTER TABLE ONLY public.endpoints
    ADD CONSTRAINT endpoints_pkey PRIMARY KEY (endpoint_id);
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_filename_prev_id_key UNIQUE (filename, prev_id);
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_pkey PRIMARY KEY (file_history_id);
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_scan_id_filename_key UNIQUE (scan_id, filename);
ALTER TABLE ONLY public.rule_results
    ADD CONSTRAINT rule_results_file_history_id_rule_id_key UNIQUE (file_history_id, rule_id);
ALTER TABLE ONLY public.rule_results
    ADD CONSTRAINT rule_results_pkey PRIMARY KEY (rule_result_id);
ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_pkey PRIMARY KEY (rule_id);
ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_principal_priority_ignore_key UNIQUE (principal, priority, ignore);
ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_principal_rule_ignore_key UNIQUE (principal, rule, ignore);
ALTER TABLE ONLY public.scans
    ADD CONSTRAINT scans_pkey PRIMARY KEY (scan_id);
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_prev_id_fkey FOREIGN KEY (prev_id) REFERENCES public.file_history(file_history_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE ONLY public.file_history
    ADD CONSTRAINT file_history_scan_id_fkey FOREIGN KEY (scan_id) REFERENCES public.scans(scan_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE ONLY public.rule_results
    ADD CONSTRAINT rule_results_file_history_id_fkey FOREIGN KEY (file_history_id) REFERENCES public.file_history(file_history_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE ONLY public.rule_results
    ADD CONSTRAINT rule_results_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.rules(rule_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
