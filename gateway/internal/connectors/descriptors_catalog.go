package connectors

// The nine catalog entries.
//
// Each one is a field model reviewed against the provider's own connection
// documentation, so the wizard asks a customer DBA for the things their console
// actually shows them. Oracle asks for a service name because Oracle has service
// names; BigQuery does not ask for a host because BigQuery has no host.
//
// Eight of the nine have no driver. They carry a StatusReason saying so, and
// `Registry.Driver` refuses them, so nothing here can produce a connection
// attempt -- let alone a successful-looking one.

// notImplementedReason is the shared explanation, phrased so an operator can
// tell "we have not built this" apart from "this is broken".
const notImplementedReason = "no driver has been implemented; the field model and capability " +
	"profile are published for review, and no connection can be created"

// commonAllowlistField is the approved-resource list every connector needs.
//
// The label differs per provider because the thing being allowlisted differs:
// PostgreSQL has schemas, BigQuery has datasets, Databricks has catalogs.
func allowlistField(label, help string) Field {
	return Field{
		ID: allowlistFieldID, Label: label, Kind: KindList, Required: true,
		Help: help,
	}
}

// tlsField builds the transport-security selector.
//
// The insecure options are present because customers have them configured and
// hiding them would make the wizard unusable against a real estate -- but each
// is marked Insecure so the UI warns and an audit can find every connection
// that chose one.
func tlsField(options []Option) Field {
	return Field{
		ID: "tls_mode", Label: "TLS mode", Kind: KindEnum, Required: true,
		Help:    "How this gateway verifies the database's identity. Full verification is the only mode that detects an interception.",
		Options: options,
	}
}

func catalogDescriptors() []Descriptor {
	return []Descriptor{
		postgresDescriptor(),
		mysqlDescriptor("mysql", "MySQL", 3306),
		mysqlDescriptor("mariadb", "MariaDB",
			// MariaDB is the same wire protocol and a separately tested
			// capability profile. It is a distinct entry rather than a flag on
			// MySQL because the divergence -- sequences, JSON handling,
			// information_schema contents -- is exactly the kind of difference
			// a shared entry would hide.
			3306),
		sqlServerDescriptor(),
		oracleDescriptor(),
		snowflakeDescriptor(),
		redshiftDescriptor(),
		bigQueryDescriptor(),
		databricksDescriptor(),
	}
}

// ---------------------------------------------------------------------------
// PostgreSQL
// ---------------------------------------------------------------------------

func postgresDescriptor() Descriptor {
	return Descriptor{
		Type:        "postgresql",
		DisplayName: "PostgreSQL",
		// Status is set by Registry.Register from conformance evidence. It
		// starts PLANNED so an entry with no driver can never be selectable
		// through a missing initialisation.
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "host", Label: "Host", Kind: KindText, Required: true,
				Placeholder: "db.internal.example.com",
				Help:        "The hostname this gateway connects to. It must resolve and be reachable from the gateway, not from your workstation."},
			{ID: "port", Label: "Port", Kind: KindNumber, Required: true, Default: "5432"},
			{ID: "database", Label: "Database", Kind: KindText, Required: true},
			{ID: "username", Label: "Username", Kind: KindText, Required: true,
				Help: "A least-privilege, read-only role. This platform never writes, so a role with write grants adds risk and no capability."},
			{ID: "password", Label: "Password", Kind: KindSecret, Required: true,
				AppliesToAuth: []string{"password"},
				Help:          "Stored write-only. It is never returned by any screen or API, and replacing it is the only way to change it."},
			{ID: "client_cert_ref", Label: "Client certificate", Kind: KindSecretRef,
				AppliesToAuth: []string{"client_certificate"},
				Help:          "Reference to a client certificate and key held in the secret store."},
			{ID: "ca_ref", Label: "CA certificate", Kind: KindSecretRef,
				Help: "Reference to the certificate authority that signs the server's certificate. Required for full verification against a private CA."},
			tlsField([]Option{
				{Value: "verify-full", Label: "Verify full (recommended)",
					Help: "Verifies the certificate chain and that the hostname matches."},
				{Value: "verify-ca", Label: "Verify CA",
					Help: "Verifies the chain but not the hostname; an attacker with any certificate from the CA can impersonate the server.", Insecure: true},
				{Value: "require", Label: "Require (encrypt only)",
					Help: "Encrypts without verifying anything, so it stops passive capture and not interception.", Insecure: true},
			}),
			allowlistField("Approved schemas",
				"Only these schemas can be discovered or queried. An empty list is refused: a connection with no allowlist would read the whole database."),
		},
		AuthModes: []AuthMode{
			{ID: "password", Label: "Password", Preferred: true},
			{ID: "client_certificate", Label: "Client certificate",
				Help: "Mutual TLS. Stronger than a password and requires the certificate to be provisioned in the secret store first."},
		},
		Template:         "postgresql://<user>:<secret>@<host>:5432/<database>?sslmode=verify-full",
		SupportsURIPaste: true,
		Capabilities: Capabilities{
			SupportsSchemas: true, SupportsCatalogs: false,
			IdentifierQuote: `"`, ParameterStyle: ParamDollar,
			ReadOnlyTransactions: true, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"verify-full", "verify-ca", "require"},
			AuthModes:       []string{"password", "client_certificate"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// ---------------------------------------------------------------------------
// MySQL and MariaDB
// ---------------------------------------------------------------------------

func mysqlDescriptor(id, name string, port int) Descriptor {
	portDefault := "3306"
	if port != 3306 {
		portDefault = itoa(port)
	}
	return Descriptor{
		Type:         id,
		DisplayName:  name,
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "host", Label: "Host", Kind: KindText, Required: true},
			{ID: "port", Label: "Port", Kind: KindNumber, Required: true, Default: portDefault},
			{ID: "database", Label: "Database", Kind: KindText, Required: true,
				Help: name + " has no schema layer separate from the database, so this is both."},
			{ID: "username", Label: "Username", Kind: KindText, Required: true},
			{ID: "password", Label: "Password", Kind: KindSecret, Required: true,
				AppliesToAuth: []string{"password"}},
			{ID: "client_cert_ref", Label: "Client certificate", Kind: KindSecretRef,
				AppliesToAuth: []string{"client_certificate"}},
			{ID: "ca_ref", Label: "CA certificate", Kind: KindSecretRef},
			tlsField([]Option{
				{Value: "verify_identity", Label: "Verify identity (recommended)",
					Help: "Verifies the chain and the hostname."},
				{Value: "verify_ca", Label: "Verify CA", Insecure: true},
				{Value: "preferred", Label: "Preferred",
					Help: "Uses TLS if the server offers it and silently continues without it if not, so it protects nothing an attacker can influence.", Insecure: true},
			}),
			allowlistField("Approved databases",
				"Only these databases can be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "password", Label: "Password", Preferred: true},
			{ID: "client_certificate", Label: "Client certificate"},
		},
		Template:         "<user>:<secret>@tcp(<host>:" + portDefault + ")/<database>?tls=<profile>",
		SupportsURIPaste: true,
		Capabilities: Capabilities{
			// No schema layer: a MySQL "database" is what PostgreSQL calls a
			// schema, and modelling it as a schema would make the allowlist
			// mean something different from what the customer selected.
			SupportsSchemas: false, SupportsCatalogs: true,
			IdentifierQuote: "`", ParameterStyle: ParamQuestion,
			ReadOnlyTransactions: true, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"verify_identity", "verify_ca", "preferred"},
			AuthModes:       []string{"password", "client_certificate"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Microsoft SQL Server
// ---------------------------------------------------------------------------

func sqlServerDescriptor() Descriptor {
	return Descriptor{
		Type:         "sqlserver",
		DisplayName:  "Microsoft SQL Server",
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "host", Label: "Server", Kind: KindText, Required: true},
			{ID: "port", Label: "Port", Kind: KindNumber, Default: "1433",
				Help: "Leave blank when connecting by named instance."},
			{ID: "instance", Label: "Instance name", Kind: KindText,
				Help: "For a named instance. Resolution uses the SQL Server Browser service, which must be reachable from this gateway."},
			{ID: "database", Label: "Database", Kind: KindText, Required: true},
			{ID: "username", Label: "Username", Kind: KindText, Required: true,
				AppliesToAuth: []string{"sql_login"}},
			{ID: "password", Label: "Password", Kind: KindSecret, Required: true,
				AppliesToAuth: []string{"sql_login"}},
			{ID: "entra_identity_ref", Label: "Entra identity", Kind: KindSecretRef,
				AppliesToAuth: []string{"entra"},
				Help:          "Reference to the managed identity or service principal held in the secret store."},
			{ID: "ca_ref", Label: "CA certificate", Kind: KindSecretRef},
			tlsField([]Option{
				{Value: "strict", Label: "Strict (recommended)",
					Help: "TDS 8.0 with full certificate validation."},
				{Value: "true", Label: "Encrypt with validation"},
				{Value: "trust_server_certificate", Label: "Encrypt, trust any certificate",
					Help: "Encrypts and accepts any certificate, which defeats the encryption against an active attacker.", Insecure: true},
			}),
			allowlistField("Approved schemas",
				"Only these schemas can be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "entra", Label: "Microsoft Entra ID", Preferred: true,
				Help: "Short-lived tokens rather than a stored password."},
			{ID: "sql_login", Label: "SQL login"},
		},
		Template:         "sqlserver://<user>:<secret>@<host>:1433?database=<database>&encrypt=true",
		SupportsURIPaste: true,
		Capabilities: Capabilities{
			SupportsSchemas: true, SupportsCatalogs: true,
			IdentifierQuote: "[", ParameterStyle: ParamAtNamed,
			ReadOnlyTransactions: true, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"strict", "true", "trust_server_certificate"},
			AuthModes:       []string{"entra", "sql_login"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Oracle
// ---------------------------------------------------------------------------

func oracleDescriptor() Descriptor {
	return Descriptor{
		Type:         "oracle",
		DisplayName:  "Oracle Database",
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "host", Label: "Host", Kind: KindText, Required: true,
				AppliesToAuth: []string{"password", "wallet"}},
			{ID: "port", Label: "Port", Kind: KindNumber, Required: true, Default: "1521"},
			{ID: "service_name", Label: "Service name", Kind: KindText, Required: true,
				Help: "The service, not the SID. An approved TNS alias may be used instead where one is configured."},
			{ID: "username", Label: "Username", Kind: KindText, Required: true},
			{ID: "password", Label: "Password", Kind: KindSecret, Required: true,
				AppliesToAuth: []string{"password"}},
			{ID: "wallet_ref", Label: "Wallet", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"wallet"},
				Help:          "Reference to the Oracle wallet held in the secret store. The wallet itself is never displayed or downloadable."},
			tlsField([]Option{
				{Value: "tcps_verify_full", Label: "TCPS with full verification (recommended)"},
				{Value: "tcps", Label: "TCPS without hostname verification", Insecure: true},
			}),
			allowlistField("Approved schemas",
				"Oracle schemas are users. Only these may be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "wallet", Label: "Wallet / mTLS", Preferred: true},
			{ID: "password", Label: "Password"},
		},
		Template:         "oracle://<user>:<secret>@<host>:1521/<service>",
		SupportsURIPaste: true,
		Capabilities: Capabilities{
			// Oracle's "schema" is a user and there is no catalog layer above
			// it in the PostgreSQL sense.
			SupportsSchemas: true, SupportsCatalogs: false,
			IdentifierQuote: `"`, ParameterStyle: ParamColonNamed,
			ReadOnlyTransactions: true, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"tcps_verify_full", "tcps"},
			AuthModes:       []string{"wallet", "password"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Snowflake
// ---------------------------------------------------------------------------

func snowflakeDescriptor() Descriptor {
	return Descriptor{
		Type:         "snowflake",
		DisplayName:  "Snowflake",
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "account", Label: "Account identifier", Kind: KindText, Required: true,
				Placeholder: "myorg-myaccount",
				Help:        "The organization-account identifier from your Snowflake console, not the full hostname."},
			{ID: "warehouse", Label: "Warehouse", Kind: KindText, Required: true,
				Help: "The compute warehouse queries run on. Use a small, dedicated one: this platform reads metadata and aggregates, and a shared warehouse makes its cost invisible."},
			{ID: "database", Label: "Database", Kind: KindText, Required: true},
			{ID: "schema", Label: "Schema", Kind: KindText, Required: true},
			{ID: "role", Label: "Role", Kind: KindText, Required: true,
				Help: "A dedicated read-only role. Snowflake resolves privileges through the active role, so this is the primary access control."},
			{ID: "username", Label: "Username", Kind: KindText, Required: true},
			{ID: "private_key_ref", Label: "Private key", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"key_pair"},
				Help:          "Reference to the RSA private key held in the secret store."},
			{ID: "oauth_token_ref", Label: "OAuth token", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"oauth"}},
			{ID: "password", Label: "Password", Kind: KindSecret, Required: true,
				AppliesToAuth: []string{"password"},
				Help:          "Local testing only. Snowflake password authentication must not be used against production data."},
			allowlistField("Approved schemas",
				"Fully qualified as database.schema. Only these may be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "key_pair", Label: "Key pair", Preferred: true},
			{ID: "oauth", Label: "OAuth"},
			{ID: "password", Label: "Password", LocalTestingOnly: true,
				Help: "Permitted for local testing only."},
		},
		// No template: forcing a JDBC-style URI on Snowflake teaches operators
		// to assemble a string when the structured parameters are the real
		// interface.
		Template:         "",
		SupportsURIPaste: false,
		Capabilities: Capabilities{
			SupportsSchemas: true, SupportsCatalogs: true,
			IdentifierQuote: `"`, ParameterStyle: ParamQuestion,
			// Snowflake has no read-only transaction mode; read-only has to
			// come from the role's grants. Declaring it false is what makes
			// that a reviewable fact rather than an assumption.
			ReadOnlyTransactions: false, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"required"},
			AuthModes:       []string{"key_pair", "oauth", "password"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Amazon Redshift
// ---------------------------------------------------------------------------

func redshiftDescriptor() Descriptor {
	return Descriptor{
		Type:         "redshift",
		DisplayName:  "Amazon Redshift",
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "host", Label: "Endpoint", Kind: KindText, Required: true,
				Help: "The cluster endpoint or Serverless workgroup endpoint."},
			{ID: "port", Label: "Port", Kind: KindNumber, Required: true, Default: "5439"},
			{ID: "database", Label: "Database", Kind: KindText, Required: true},
			{ID: "region", Label: "AWS region", Kind: KindText, Required: true,
				Help: "Required for IAM credential generation."},
			{ID: "username", Label: "Database user", Kind: KindText, Required: true},
			{ID: "iam_role_ref", Label: "IAM role", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"iam"},
				Help:          "Reference to the role this gateway assumes. Temporary credentials are generated per connection and never stored."},
			{ID: "password", Label: "Password", Kind: KindSecret, Required: true,
				AppliesToAuth: []string{"password"}},
			tlsField([]Option{
				{Value: "verify-full", Label: "Verify full (recommended)"},
				{Value: "require", Label: "Require (encrypt only)", Insecure: true},
			}),
			allowlistField("Approved schemas",
				"Only these schemas can be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "iam", Label: "IAM temporary credentials", Preferred: true,
				Help: "Short-lived credentials, so a leak expires."},
			{ID: "password", Label: "Password"},
		},
		Template:         "postgresql://<user>:<secret>@<endpoint>:5439/<database>?sslmode=verify-full",
		SupportsURIPaste: true,
		Capabilities: Capabilities{
			// Wire-compatible with PostgreSQL and a genuinely different engine:
			// no foreign keys enforced, different system catalogs, different
			// concurrency behaviour. A separate profile, not an alias.
			SupportsSchemas: true, SupportsCatalogs: false,
			IdentifierQuote: `"`, ParameterStyle: ParamDollar,
			ReadOnlyTransactions: true, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"verify-full", "require"},
			AuthModes:       []string{"iam", "password"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Google BigQuery
// ---------------------------------------------------------------------------

func bigQueryDescriptor() Descriptor {
	return Descriptor{
		Type:         "bigquery",
		DisplayName:  "Google BigQuery",
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "project", Label: "Project", Kind: KindText, Required: true,
				Help: "The project containing the data."},
			{ID: "billing_project", Label: "Billing project", Kind: KindText, Required: true,
				Help: "The project charged for query execution. Separating it from the data project keeps this platform's cost attributable."},
			{ID: "location", Label: "Location", Kind: KindText, Required: true,
				Placeholder: "US",
				Help:        "Dataset location. A query against a dataset in another location fails rather than moving data."},
			{ID: "workload_identity_ref", Label: "Workload identity", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"workload_identity"},
				Help:          "Reference to the federated identity configuration. No key material is stored."},
			{ID: "service_account_ref", Label: "Service account key", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"service_account"},
				Help:          "Reference to the service-account JSON held in the secret store. The JSON is never displayed or downloadable."},
			allowlistField("Approved datasets",
				"Only these datasets can be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "workload_identity", Label: "Workload identity federation", Preferred: true,
				Help: "No long-lived key material exists to leak."},
			{ID: "service_account", Label: "Service account key",
				Help: "A long-lived credential. Prefer workload identity where the deployment supports it."},
		},
		// No URI and no paste. BigQuery has no host, port or password, so
		// offering a connection-string box would invite an operator to paste
		// something that is not one -- most likely the service-account JSON,
		// into a field that is not a secret field.
		Template:         "",
		SupportsURIPaste: false,
		Capabilities: Capabilities{
			SupportsSchemas: true, SupportsCatalogs: true,
			IdentifierQuote: "`", ParameterStyle: ParamNamed,
			// No transactions in the interactive query API, so read-only comes
			// from IAM. Stated rather than assumed.
			ReadOnlyTransactions: false, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"required"},
			AuthModes:       []string{"workload_identity", "service_account"},
			MetadataQueries: true, AggregateQueries: true,
			// Byte-scanned is BigQuery's cost model, so the byte limit is the
			// meaningful bound rather than the row count.
			MaxBytes: 1 << 30,
		},
	}
}

// ---------------------------------------------------------------------------
// Databricks SQL
// ---------------------------------------------------------------------------

func databricksDescriptor() Descriptor {
	return Descriptor{
		Type:         "databricks",
		DisplayName:  "Databricks SQL",
		Status:       StatusPlanned,
		StatusReason: notImplementedReason,
		Fields: []Field{
			{ID: "workspace_host", Label: "Workspace host", Kind: KindText, Required: true,
				Placeholder: "dbc-00000000-0000.cloud.databricks.com"},
			{ID: "http_path", Label: "HTTP path", Kind: KindText, Required: true,
				Placeholder: "/sql/1.0/warehouses/0000000000000000",
				Help:        "The SQL warehouse's HTTP path, from the warehouse's connection details."},
			{ID: "catalog", Label: "Catalog", Kind: KindText, Required: true,
				Help: "Unity Catalog name."},
			{ID: "schema", Label: "Schema", Kind: KindText, Required: true},
			{ID: "oauth_client_ref", Label: "Service principal", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"oauth_m2m"},
				Help:          "Reference to the OAuth service principal credentials held in the secret store."},
			{ID: "pat_ref", Label: "Personal access token", Kind: KindSecretRef, Required: true,
				AppliesToAuth: []string{"pat"},
				Help:          "Reference only. A personal access token is tied to a person and outlives their access to the workspace; prefer a service principal."},
			allowlistField("Approved schemas",
				"Fully qualified as catalog.schema. Only these may be discovered or queried."),
		},
		AuthModes: []AuthMode{
			{ID: "oauth_m2m", Label: "OAuth service principal", Preferred: true},
			{ID: "pat", Label: "Personal access token",
				Help: "Tied to an individual. Prefer a service principal."},
		},
		Template:         "",
		SupportsURIPaste: false,
		Capabilities: Capabilities{
			SupportsSchemas: true, SupportsCatalogs: true,
			IdentifierQuote: "`", ParameterStyle: ParamQuestion,
			ReadOnlyTransactions: false, StatementTimeout: true,
			Cancellation: true, CursorPagination: true,
			TLSModes:        []string{"required"},
			AuthModes:       []string{"oauth_m2m", "pat"},
			MetadataQueries: true, AggregateQueries: true,
		},
	}
}

// itoa avoids importing strconv for one call in a descriptor definition.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
