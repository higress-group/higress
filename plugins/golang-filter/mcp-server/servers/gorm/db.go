package gorm

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DBClient is a struct to handle database connections and operations
type DBClient struct {
	db         *gorm.DB
	dsn        string
	dbType     string
	reconnect  chan struct{}
	stop       chan struct{}
	panicCount int32 // Add panic counter
}

// supports database types
const (
	MYSQL      = "mysql"
	POSTGRES   = "postgres"
	CLICKHOUSE = "clickhouse"
	SQLITE     = "sqlite"
)

// NewDBClient creates a new DBClient instance and establishes a connection to the database
func NewDBClient(dsn string, dbType string, stop chan struct{}) *DBClient {
	client := &DBClient{
		dsn:       dsn,
		dbType:    dbType,
		reconnect: make(chan struct{}, 1),
		stop:      stop,
	}

	// Start reconnection goroutine
	go client.reconnectLoop()

	// Try initial connection
	if err := client.connect(); err != nil {
		api.LogErrorf("Initial database connection failed: %v", err)
	}

	return client
}

func (c *DBClient) connect() error {
	var db *gorm.DB
	var err error
	gormConfig := gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	switch c.dbType {
	case POSTGRES:
		db, err = gorm.Open(postgres.Open(c.dsn), &gormConfig)
	case CLICKHOUSE:
		db, err = gorm.Open(clickhouse.Open(c.dsn), &gormConfig)
	case MYSQL:
		db, err = gorm.Open(mysql.Open(c.dsn), &gormConfig)
	case SQLITE:
		db, err = gorm.Open(sqlite.Open(c.dsn), &gormConfig)
	default:
		return fmt.Errorf("unsupported database type %s", c.dbType)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	c.db = db
	return nil
}

func (c *DBClient) reconnectLoop() {
	defer func() {
		if r := recover(); r != nil {
			api.LogErrorf("Recovered from panic in reconnectLoop: %v", r)

			// Increment panic counter
			atomic.AddInt32(&c.panicCount, 1)

			// If panic count exceeds threshold, stop trying to reconnect
			if atomic.LoadInt32(&c.panicCount) > 3 {
				api.LogErrorf("Too many panics in reconnectLoop, stopping reconnection attempts")
				return
			}

			// Wait for a while before restarting
			time.Sleep(5 * time.Second)

			// Restart the reconnect loop
			go c.reconnectLoop()
		}
	}()

	ticker := time.NewTicker(30 * time.Second) // Try to reconnect every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			api.LogInfof("Database %s connection closed", c.dbType)
			return
		case <-ticker.C:
			if c.db == nil || c.Ping() != nil {
				if err := c.connect(); err != nil {
					api.LogErrorf("Database reconnection failed: %v", err)
				} else {
					api.LogInfof("Database reconnected successfully")
					// Reset panic count on successful connection
					atomic.StoreInt32(&c.panicCount, 0)
				}
			}
		case <-c.reconnect:
			if err := c.connect(); err != nil {
				api.LogErrorf("Database reconnection failed: %v", err)
			} else {
				api.LogInfof("Database reconnected successfully")
				// Reset panic count on successful connection
				atomic.StoreInt32(&c.panicCount, 0)
			}
		}
	}
}

func (c *DBClient) reconnectIfDbEmpty() error {
	if c.db == nil {
		// Trigger reconnection
		select {
		case c.reconnect <- struct{}{}:
		default:
		}
		return fmt.Errorf("database is not connected, attempting to reconnect")
	}
	return nil
}

func (c *DBClient) handleSQLError(err error) error {
	if err != nil {
		// If execution fails, connection might be lost, trigger reconnection
		select {
		case c.reconnect <- struct{}{}:
		default:
		}
		return fmt.Errorf("failed to execute SQL: %w", err)
	}
	return nil
}

// DescribeTable Get the structure of a specific table.
func (c *DBClient) DescribeTable(table string) ([]map[string]interface{}, error) {
	var sql string
	switch c.dbType {
	case MYSQL:
		sql = `
			select
			    column_name,
				column_type,
				is_nullable,
				column_key,
				column_default,
				extra,
				column_comment
			from information_schema.columns
			where table_schema = database() and table_name = ?
		`
		return c.Query(sql, table)

	case POSTGRES:
		sql = `
			select
			    column_name,
				data_type as column_type,
				is_nullable,
				case
				    when column_default like 'nextval%' then 'auto_increment'
				    when column_default is not null then 'default'
				    else ''
				end as column_key,
				column_default,
				case
				    when column_default like 'nextval%' then 'auto_increment'
				    else ''
				end as extra,
				col_description((select oid from pg_class where relname = ?), ordinal_position) as column_comment
			from information_schema.columns
			where table_name = ?
		`
		lowerTable := strings.ToLower(table)
		return c.Query(sql, lowerTable, lowerTable)

	case CLICKHOUSE:
		sql = `
			select
			    name as column_name,
				type as column_type,
				if(is_nullable, 'YES', 'NO') as is_nullable,
				default_kind as column_key,
				default_expression as column_default,
				default_kind as extra,
				comment as column_comment
			from system.columns
			where database = currentDatabase() and table = ?
		`
		return c.Query(sql, table)

	case SQLITE:
		sql = `
			select
			    name as column_name,
				type as column_type,
				not (notnull = 1) as is_nullable,
				pk as column_key,
				dflt_value as column_default,
				'' as extra,
				'' as column_comment
			from pragma_table_info(?)
		`
		return c.Query(sql, table)

	default:
		return nil, fmt.Errorf("unsupported database type: %s", c.dbType)
	}
}

// ListTables List all tables in the connected database.
func (c *DBClient) ListTables() ([]string, error) {
	if err := c.reconnectIfDbEmpty(); err != nil {
		return nil, err
	}

	var sql string
	switch c.dbType {
	case MYSQL:
		sql = "show tables"
	case POSTGRES:
		sql = "select tablename from pg_tables where schemaname = 'public'"
	case CLICKHOUSE:
		sql = "select name from system.tables where database = currentDatabase()"
	case SQLITE:
		sql = "select name from sqlite_master where type='table'"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", c.dbType)
	}

	rows, err := c.db.Raw(sql).Rows()
	if err := c.handleSQLError(err); err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table rows: %w", err)
	}

	return tables, nil
}

// Execute executes an INSERT, UPDATE, or DELETE raw SQL and returns the rows affected
func (c *DBClient) Execute(sql string, args ...interface{}) (int64, error) {
	if err := c.reconnectIfDbEmpty(); err != nil {
		return 0, err
	}

	tx := c.db.Exec(sql, args...)
	if err := c.handleSQLError(tx.Error); err != nil {
		return 0, err
	}
	defer tx.Commit()

	return tx.RowsAffected, nil
}

// Query executes a raw SQL query and returns the result as a slice of maps
func (c *DBClient) Query(sql string, args ...interface{}) ([]map[string]interface{}, error) {
	if err := c.reconnectIfDbEmpty(); err != nil {
		return nil, err
	}

	rows, err := c.db.Raw(sql, args...).Rows()
	if err := c.handleSQLError(err); err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Prepare a slice to hold the results
	var results []map[string]interface{}

	// Iterate over the rows
	for rows.Next() {
		// Create a slice of interface{}'s to represent each column,
		// and a second slice to contain pointers to each item in the columns slice.
		columnsData := make([]interface{}, len(columns))
		columnsPointers := make([]interface{}, len(columns))
		for i := range columnsData {
			columnsPointers[i] = &columnsData[i]
		}

		// Scan the result into the column pointers...
		if err := rows.Scan(columnsPointers...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map to hold the column name and value
		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			val := columnsData[i]
			b, ok := val.([]byte)
			if ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}

		// Append the map to the results slice
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate table rows in Query: %w", err)
	}

	return results, nil
}

func (c *DBClient) Ping() error {
	if c.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Use context to set timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Try to ping the database
	sqlDB, err := c.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying *sql.DB: %v", err)
	}

	return sqlDB.PingContext(ctx)
}
