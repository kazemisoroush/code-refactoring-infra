"""
Test suite for simplified Lambda handler that creates Postgres table with vector schema.
"""
import unittest
from unittest.mock import patch, MagicMock
import json
import os
from botocore.exceptions import ClientError
import psycopg2

import handler


class TestGetSecretValue(unittest.TestCase):
    """Test get_secret_value function."""

    @patch("boto3.client")
    def test_get_secret_value_success(self, mock_boto_client):
        """Should return secret data when SecretString is present."""
        mock_secrets = MagicMock()
        mock_boto_client.return_value = mock_secrets

        secret_data = {"username": "test-user", "password": "test-pass"}
        mock_secrets.get_secret_value.return_value = {
            "SecretString": json.dumps(secret_data)
        }

        result = handler.get_secret_value("arn:aws:secretsmanager:xyz")
        self.assertEqual(result["username"], "test-user")
        self.assertEqual(result["password"], "test-pass")

    @patch("boto3.client")
    def test_get_secret_value_client_error(self, mock_boto_client):
        """Should raise exception when Secrets Manager call fails."""
        mock_secrets = MagicMock()
        mock_boto_client.return_value = mock_secrets
        mock_secrets.get_secret_value.side_effect = ClientError(
            {"Error": {"Code": "AccessDeniedException", "Message": "Denied!"}},
            "GetSecretValue"
        )

        with self.assertRaises(ClientError):
            handler.get_secret_value("arn:denied")


class TestCreateDatabaseIfNotExists(unittest.TestCase):
    """Test create_database_if_not_exists function."""

    @patch("handler.psycopg2.connect")
    def test_create_database_if_not_exists_already_exists(self, mock_connect):
        """Should not create database when it already exists."""
        mock_conn = MagicMock()
        mock_connect.return_value = mock_conn
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        # Mock database exists
        mock_cursor.fetchone.return_value = [1]

        admin_db_config = {
            "host": "localhost",
            "port": 5432,
            "dbname": "postgres",
            "username": "user",
            "password": "pass"
        }

        handler.create_database_if_not_exists(admin_db_config, "test_db")

        # Should check if database exists
        mock_cursor.execute.assert_called_once_with(
            "SELECT 1 FROM pg_database WHERE datname = %s",
            ("test_db",)
        )

        # Should not create database since it exists
        create_calls = [call for call in mock_cursor.execute.call_args_list
                       if "CREATE DATABASE" in str(call)]
        self.assertEqual(len(create_calls), 0)

    @patch("handler.psycopg2.connect")
    def test_create_database_if_not_exists_creates_new(self, mock_connect):
        """Should create database when it doesn't exist."""
        mock_conn = MagicMock()
        mock_connect.return_value = mock_conn
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        # Mock database doesn't exist
        mock_cursor.fetchone.return_value = None

        admin_db_config = {
            "host": "localhost",
            "port": 5432,
            "dbname": "postgres",
            "username": "user",
            "password": "pass"
        }

        handler.create_database_if_not_exists(admin_db_config, "test_db")

        # Should check if database exists and then create it
        self.assertEqual(mock_cursor.execute.call_count, 2)

        # Check first call (existence check)
        first_call = mock_cursor.execute.call_args_list[0]
        self.assertEqual(first_call[0][0], "SELECT 1 FROM pg_database WHERE datname = %s")
        self.assertEqual(first_call[0][1], ("test_db",))

        # Check second call (database creation)
        second_call = mock_cursor.execute.call_args_list[1][0][0]
        self.assertEqual(second_call, 'CREATE DATABASE "test_db"')

        # Should set autocommit
        self.assertTrue(mock_conn.autocommit)


class TestCreateTableAndIndexes(unittest.TestCase):
    """Test create_table_and_indexes function."""

    @patch.dict(os.environ, {"EMBEDDING_DIMENSIONS": "1536"})
    @patch("handler.psycopg2.connect")
    def test_create_table_and_indexes_success(self, mock_connect):
        """Should create table and indexes successfully."""
        mock_conn = MagicMock()
        mock_connect.return_value = mock_conn
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        db_config = {
            "host": "localhost",
            "port": 5432,
            "dbname": "testdb",
            "username": "user",
            "password": "pass"
        }
        handler.create_table_and_indexes(db_config, "my_table")

        # Should execute: extension, table, text index, vector index
        self.assertEqual(mock_cursor.execute.call_count, 4)

        # Check extension creation
        first_call = mock_cursor.execute.call_args_list[0][0][0]
        self.assertIn("CREATE EXTENSION IF NOT EXISTS vector", first_call)

        # Check table creation
        second_call = mock_cursor.execute.call_args_list[1][0][0]
        self.assertIn('CREATE TABLE IF NOT EXISTS "my_table"', second_call)
        self.assertIn("id UUID PRIMARY KEY", second_call)
        self.assertIn("vector(1536)", second_call)
        self.assertIn("metadata JSONB", second_call)

        # Check text index creation
        third_call = mock_cursor.execute.call_args_list[2][0][0]
        self.assertIn("CREATE INDEX IF NOT EXISTS", third_call)
        self.assertIn("gin", third_call)
        self.assertIn("to_tsvector", third_call)

        # Check vector index creation (HNSW)
        fourth_call = mock_cursor.execute.call_args_list[3][0][0]
        self.assertIn("CREATE INDEX IF NOT EXISTS", fourth_call)
        self.assertIn("hnsw", fourth_call)
        self.assertIn("vector_cosine_ops", fourth_call)

        mock_conn.commit.assert_called_once()
        mock_conn.close.assert_called_once()

    @patch.dict(os.environ, {"EMBEDDING_DIMENSIONS": "1024"})
    @patch("handler.psycopg2.connect")
    def test_create_table_custom_dimensions(self, mock_connect):
        """Should use custom embedding dimensions from environment."""
        mock_conn = MagicMock()
        mock_connect.return_value = mock_conn
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        db_config = {
            "host": "localhost",
            "port": 5432,
            "dbname": "testdb",
            "username": "user",
            "password": "pass"
        }
        handler.create_table_and_indexes(db_config, "my_table")

        # Check that custom dimensions are used
        second_call = mock_cursor.execute.call_args_list[1][0][0]
        self.assertIn("vector(1024)", second_call)

    @patch("handler.psycopg2.connect")
    def test_create_table_hnsw_fallback_to_ivfflat(self, mock_connect):
        """Should fallback to IVFFlat when HNSW is not available."""
        mock_conn = MagicMock()
        mock_connect.return_value = mock_conn
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        # Mock HNSW failure
        def mock_execute_side_effect(sql):
            if "hnsw" in sql:
                raise psycopg2.Error("access method hnsw does not exist")

        mock_cursor.execute.side_effect = mock_execute_side_effect

        db_config = {
            "host": "localhost",
            "port": 5432,
            "dbname": "testdb",
            "username": "user",
            "password": "pass"
        }
        handler.create_table_and_indexes(db_config, "my_table")

        # Should execute: extension, table, text index, failed HNSW, successful IVFFlat
        self.assertEqual(mock_cursor.execute.call_count, 5)

        # Check that IVFFlat index was created
        fifth_call = mock_cursor.execute.call_args_list[4][0][0]
        self.assertIn("CREATE INDEX IF NOT EXISTS", fifth_call)
        self.assertIn("ivfflat", fifth_call)
        self.assertIn("vector_cosine_ops", fifth_call)
        self.assertIn("lists = 100", fifth_call)

    @patch("handler.psycopg2.connect")
    def test_create_table_vector_index_failure(self, mock_connect):
        """Should handle vector index creation failure gracefully."""
        mock_conn = MagicMock()
        mock_connect.return_value = mock_conn
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

        # Mock vector index failure with unexpected error
        def mock_execute_side_effect(sql):
            if "hnsw" in sql or "ivfflat" in sql:
                raise psycopg2.Error("unexpected vector index error")

        mock_cursor.execute.side_effect = mock_execute_side_effect

        # Should not raise exception
        db_config = {
            "host": "localhost",
            "port": 5432,
            "dbname": "testdb",
            "username": "user",
            "password": "pass"
        }
        handler.create_table_and_indexes(db_config, "my_table")

        mock_conn.commit.assert_called_once()
        mock_conn.close.assert_called_once()


class TestLambdaHandler(unittest.TestCase):
    """Test lambda_handler function."""

    @patch.dict(os.environ, {
        "DB_HOST": "localhost",
        "DB_PORT": "5432",
        "DB_NAME": "testdb",
        "DB_SECRET_ARN": "arn:secret"
    })
    @patch("handler.get_secret_value")
    @patch("handler.create_database_if_not_exists")
    @patch("handler.create_table_and_indexes")
    def test_lambda_handler_success(self, mock_create_table, mock_create_db, mock_get_secret):
        """Should return success when all operations complete successfully."""
        mock_get_secret.return_value = {"username": "user", "password": "pass"}

        event = {"table": "my_table", "database": "my_database"}
        result = handler.lambda_handler(event, {})

        self.assertEqual(result["status"], "success")
        self.assertIn("Database my_database and table my_table created successfully", result["message"])

        # Verify functions were called
        mock_get_secret.assert_called_once_with("arn:secret")

        # Verify create_database_if_not_exists was called with admin config
        mock_create_db.assert_called_once_with(
            {
                "host": "localhost",
                "port": 5432,
                "dbname": "testdb",  # admin database
                "username": "user",
                "password": "pass"
            },
            "my_database"
        )

        # Verify create_table_and_indexes was called with target database config
        mock_create_table.assert_called_once_with(
            {
                "host": "localhost",
                "port": 5432,
                "dbname": "my_database",  # target database
                "username": "user",
                "password": "pass"
            },
            "my_table"
        )

    def test_lambda_handler_missing_table(self):
        """Should return error when 'table' is missing in event."""
        event = {"database": "my_database"}  # No "table" key
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("Missing 'table' in event", result["message"])

    def test_lambda_handler_missing_database(self):
        """Should return error when 'database' is missing in event."""
        event = {"table": "my_table"}  # No "database" key
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("Missing 'database' in event", result["message"])

    def test_lambda_handler_empty_table(self):
        """Should return error when 'table' is empty."""
        event = {"table": "", "database": "my_database"}
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("Missing 'table' in event", result["message"])

    def test_lambda_handler_empty_database(self):
        """Should return error when 'database' is empty."""
        event = {"table": "my_table", "database": ""}
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("Missing 'database' in event", result["message"])

    @patch.dict(os.environ, {}, clear=True)
    def test_lambda_handler_missing_env_vars(self):
        """Should return error when required environment variables are missing."""
        event = {"table": "my_table"}
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        # Should contain KeyError information about missing env var

    @patch.dict(os.environ, {
        "DB_HOST": "localhost",
        "DB_PORT": "5432",
        "DB_NAME": "testdb",
        "DB_SECRET_ARN": "arn:secret"
    })
    @patch("handler.get_secret_value", side_effect=ClientError(
        {"Error": {"Code": "AccessDeniedException", "Message": "Denied!"}},
        "GetSecretValue"
    ))
    def test_lambda_handler_secret_error(self, _mock_get_secret):
        """Should return error when secret retrieval fails."""
        event = {"table": "my_table", "database": "my_database"}
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("AccessDeniedException", result["message"])

    @patch.dict(os.environ, {
        "DB_HOST": "localhost",
        "DB_PORT": "5432",
        "DB_NAME": "testdb",
        "DB_SECRET_ARN": "arn:secret"
    })
    @patch("handler.get_secret_value")
    @patch("handler.create_database_if_not_exists", side_effect=psycopg2.Error("Connection failed"))
    def test_lambda_handler_database_error(self, _mock_create_db, mock_get_secret):
        """Should return error when database operations fail."""
        mock_get_secret.return_value = {"username": "user", "password": "pass"}

        event = {"table": "my_table", "database": "my_database"}
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("Connection failed", result["message"])

    @patch.dict(os.environ, {
        "DB_HOST": "localhost",
        "DB_PORT": "5432",
        "DB_NAME": "testdb",
        "DB_SECRET_ARN": "arn:secret"
    })
    @patch("handler.get_secret_value")
    @patch("handler.create_database_if_not_exists")
    @patch("handler.create_table_and_indexes", side_effect=psycopg2.Error("Table creation failed"))
    def test_lambda_handler_table_creation_error(self, _mock_create_table, _mock_create_db, mock_get_secret):
        """Should return error when table creation fails."""
        mock_get_secret.return_value = {"username": "user", "password": "pass"}

        event = {"table": "my_table", "database": "my_database"}
        result = handler.lambda_handler(event, {})
        self.assertEqual(result["status"], "error")
        self.assertIn("Table creation failed", result["message"])


if __name__ == '__main__':
    unittest.main()
