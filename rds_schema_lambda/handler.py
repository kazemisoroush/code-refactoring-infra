"""
Lambda to create Postgres table with vector schema and indexes.
"""
import json
import os
import boto3
import psycopg2
from botocore.exceptions import ClientError


def get_secret_value(secret_arn):
    """Get database credentials from Secrets Manager."""
    print(f"Fetching secret from Secrets Manager: {secret_arn}")
    client = boto3.client("secretsmanager")
    response = client.get_secret_value(SecretId=secret_arn)
    return json.loads(response["SecretString"])


def create_database_if_not_exists(admin_db_config, target_db_name):
    """Create database if it doesn't exist."""
    print(f"Checking if database {target_db_name} exists...")

    conn = psycopg2.connect(
        host=admin_db_config["host"],
        port=admin_db_config["port"],
        dbname=admin_db_config["dbname"],  # Connect to default postgres db
        user=admin_db_config["username"],
        password=admin_db_config["password"],
        connect_timeout=10
    )

    try:
        conn.autocommit = True  # Required for CREATE DATABASE
        with conn.cursor() as cursor:
            # Check if database exists
            cursor.execute(
                "SELECT 1 FROM pg_database WHERE datname = %s",
                (target_db_name,)
            )
            if cursor.fetchone():
                print(f"Database {target_db_name} already exists")
            else:
                # Create database
                cursor.execute(f'CREATE DATABASE "{target_db_name}"')
                print(f"Created database {target_db_name}")
    finally:
        conn.close()


def create_table_and_indexes(db_config, table_name):
    """Create table and indexes if they don't exist."""
    print(f"Creating table and indexes for: {table_name}")

    # Get embedding dimensions from environment variable
    embedding_dimensions = os.getenv("EMBEDDING_DIMENSIONS", "1536")

    conn = psycopg2.connect(
        host=db_config["host"],
        port=db_config["port"],
        dbname=db_config["dbname"],
        user=db_config["username"],
        password=db_config["password"],
        connect_timeout=10
    )

    try:
        with conn.cursor() as cursor:
            # Enable vector extension
            cursor.execute("CREATE EXTENSION IF NOT EXISTS vector;")

            # Create table if it doesn't exist
            # Table name is pre-sanitized by the Go code to be a valid SQL identifier
            create_table_sql = f"""
                CREATE TABLE IF NOT EXISTS "{table_name}" (
                    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                    text TEXT,
                    embedding vector({embedding_dimensions}),
                    metadata JSONB
                )
            """
            cursor.execute(create_table_sql)
            print(f"Table {table_name} created or already exists")

            # Create text search index
            create_text_index_sql = f"""
                CREATE INDEX IF NOT EXISTS "{table_name}_text_gin_idx"
                ON "{table_name}" USING gin (to_tsvector('simple', text))
            """
            cursor.execute(create_text_index_sql)
            print(f"Text search index created for {table_name}")

            # Create vector index - try HNSW first, fallback to IVFFlat
            try:
                create_vector_index_sql = f"""
                    CREATE INDEX IF NOT EXISTS "{table_name}_embedding_hnsw_idx"
                    ON "{table_name}" USING hnsw (embedding vector_cosine_ops)
                """
                cursor.execute(create_vector_index_sql)
                print(f"HNSW vector index created for {table_name}")
            except psycopg2.Error as e:
                if "access method" in str(e) and "hnsw" in str(e):
                    print(f"HNSW not available, using IVFFlat: {e}")
                    create_vector_index_sql = f"""
                        CREATE INDEX IF NOT EXISTS "{table_name}_embedding_ivfflat_idx"
                        ON "{table_name}" USING ivfflat (embedding vector_cosine_ops)
                        WITH (lists = 100)
                    """
                    cursor.execute(create_vector_index_sql)
                    print(f"IVFFlat vector index created for {table_name}")
                else:
                    print(f"Vector index creation failed: {e}")

        conn.commit()
        print(f"Successfully created table and indexes for {table_name}")

    finally:
        conn.close()


def lambda_handler(event, _context):
    """Lambda handler function."""
    try:
        print("Received event:", json.dumps(event, indent=2))

        # Get table name and database name from event
        table_name = event.get("table")
        if not table_name:
            raise ValueError("Missing 'table' in event")

        target_db_name = event.get("database")
        if not target_db_name:
            raise ValueError("Missing 'database' in event")

        # Get configuration from environment
        db_host = os.environ["DB_HOST"]
        db_port = int(os.environ["DB_PORT"])
        default_db_name = os.environ["DB_NAME"]  # This is the default cluster database
        secret_arn = os.environ["DB_SECRET_ARN"]

        # Get database credentials
        secret_data = get_secret_value(secret_arn)

        # First, create the target database if it doesn't exist
        admin_db_config = {
            "host": db_host,
            "port": db_port,
            "dbname": default_db_name,  # Connect to default database first
            "username": secret_data["username"],
            "password": secret_data["password"]
        }
        create_database_if_not_exists(admin_db_config, target_db_name)

        # Then create table and indexes in the target database
        target_db_config = {
            "host": db_host,
            "port": db_port,
            "dbname": target_db_name,  # Connect to target database
            "username": secret_data["username"],
            "password": secret_data["password"]
        }
        create_table_and_indexes(target_db_config, table_name)

        return {
            "status": "success",
            "message": f"Database {target_db_name} and table {table_name} created successfully"
        }

    except (ValueError, KeyError, psycopg2.Error, ClientError) as e:
        print(f"Error: {e}")
        return {
            "status": "error",
            "message": str(e)
        }
