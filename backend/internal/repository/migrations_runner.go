package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/migrations"
)

// schemaMigrationsTableDDL 定义迁移记录表的 DDL。
// 该表用于跟踪已应用的迁移文件及其校验和。
// - filename: 迁移文件名，作为主键唯一标识每个迁移
// - checksum: 文件内容的 SHA256 哈希值，用于检测迁移文件是否被篡改
// - applied_at: 迁移应用时间戳
const schemaMigrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename   TEXT PRIMARY KEY,
	checksum   TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

const atlasSchemaRevisionsTableDDL = `
CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
	version TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	type INTEGER NOT NULL,
	applied INTEGER NOT NULL DEFAULT 0,
	total INTEGER NOT NULL DEFAULT 0,
	executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	execution_time BIGINT NOT NULL DEFAULT 0,
	error TEXT NULL,
	error_stmt TEXT NULL,
	hash TEXT NOT NULL DEFAULT '',
	partial_hashes TEXT[] NULL,
	operator_version TEXT NULL
);
`

// migrationsAdvisoryLockID 是用于序列化迁移操作的 PostgreSQL Advisory Lock ID。
// 在多实例部署场景下，该锁确保同一时间只有一个实例执行迁移。
// 任何稳定的 int64 值都可以，只要不与同一数据库中的其他锁冲突即可。
const migrationsAdvisoryLockID int64 = 694208311321144027
const migrationsLockRetryInterval = 500 * time.Millisecond
const nonTransactionalMigrationSuffix = "_notx.sql"
const paymentOrdersOutTradeNoUniqueMigration = "120_enforce_payment_orders_out_trade_no_unique_notx.sql"
const paymentOrdersOutTradeNoUniqueIndex = "paymentorder_out_trade_no_unique"
const schedulerOutboxPendingDedupKeyMigration = "153_scheduler_outbox_pending_dedup_key_index_notx.sql"
const schedulerOutboxPendingDedupKeyIndex = "idx_scheduler_outbox_pending_dedup_key"
const accountSparkShadowIndexesMigration = "154a_account_spark_shadow_indexes_notx.sql"
const accountParentAccountIDIndex = "idx_accounts_parent_account_id"
const accountSparkShadowPerParentIndex = "uq_accounts_spark_shadow_per_parent"
const latestAPIKeyIPIndexMigration = "174_add_usage_logs_api_key_latest_ip_index_notx.sql"
const latestAPIKeyIPIndex = "idx_usage_logs_api_key_latest_ip"
const usageLogsUpstreamModelMismatchIndexMigration = "195_add_usage_log_upstream_model_mismatch_index_notx.sql"
const usageLogsUpstreamModelMismatchIndex = "idx_usage_logs_upstream_model_mismatch_created_at"

type migrationChecksumCompatibilityRule struct {
	fileChecksum       string
	acceptedDBChecksum map[string]struct{}
}

// migrationChecksumCompatibilityRules 仅用于兼容历史上误修改过的迁移文件 checksum。
// 规则必须同时匹配「迁移名 + 已知数据库 checksum + 唯一规范文件 checksum」才会放行：
// 数据库可保留历史二开 checksum，但工作树中的 migration 必须保持当前规范内容，
// 避免兼容规则反过来允许旧文件或误改文件重新进入发布包。
var migrationChecksumCompatibilityRules = map[string]migrationChecksumCompatibilityRule{
	"006_add_users_allowed_groups_compat.sql":                 newMigrationChecksumCompatibilityRule("900f5ba934e8d66bba7d94f1d34463a9022e0e72c3ce911260d7c703449a33ae", "c1301d7c0d9cb7a25e7a00ddc930992a6b645270d4379fd6dac023f847861169"),
	"006_fix_invalid_subscription_expires_at.sql":             newMigrationChecksumCompatibilityRule("ed5d6553a86578d987088331bc8810808b75e554a160b66ff4626815ea838354", "4ca60e82e381d97f19f4a6f1046a77c7d147a727553f0aea9417c3674267c0d3"),
	"006b_guard_users_allowed_groups.sql":                     newMigrationChecksumCompatibilityRule("6953b71b92d0ed2f035137fdc4e4fb6cc056861b22e28c5612950c08cedf564e", "cb97c4944338921bd9cdbba5eee7071abd775f67456798716637e0e1575b0de6"),
	"009_fix_usage_logs_cache_columns.sql":                    newMigrationChecksumCompatibilityRule("9a3c22b296ea5a628eb8b35b34318f035b9ac177dfc7d1db26b0901dcd98bafb", "e2127eb45db1fdd6c717aa77227831e542457be95bff8f7f28669197853b060d"),
	"054_drop_legacy_cache_columns.sql":                       newMigrationChecksumCompatibilityRule("82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d", "182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4"),
	"061_add_usage_log_request_type.sql":                      newMigrationChecksumCompatibilityRule("66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c", "08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0", "222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3"),
	"108a_widen_auth_identity_migration_report_type.sql":      newMigrationChecksumCompatibilityRule("87646a866b17a329b01abb241b99e3874ee2c13c7d7bba3f6e37c53722a60e18", "ce0461cef9871b3042eefebc6065c94a21d61c339aa82eb44d95c19e5f4d9062"),
	"109_auth_identity_compat_backfill.sql":                   newMigrationChecksumCompatibilityRule("2b380305e73ff0c13aa8c811e45897f2b36ca4a438f7b3e8f98e19ecb6bae0b3", "748ddcdc60f93a1ac562ce8a66ee870f64ee594bf6dbedad55ed8baf3c75b28c"),
	"110_pending_auth_and_provider_default_grants.sql":        newMigrationChecksumCompatibilityRule("57a196a9810fb478fa001dfff110f5c76a7d87fb04f15e12e513fcb75402d7a6", "301e90405b3424967b7d1931568b7a244902148fa82802f362c115ae4e2ae2ef"),
	"112_add_payment_order_provider_key_snapshot.sql":         newMigrationChecksumCompatibilityRule("ab871fc02da1eabe0de6ca74a119ee3cea9c727caed30af2ae07a0cd1176d1b8", "d4476c67ceea871aa2d92ee2a603795a742d0379a58cf53938bb9aa559ff9caa"),
	"115_auth_identity_legacy_external_backfill.sql":          newMigrationChecksumCompatibilityRule("022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f", "022370762c2dd0ce4f665579b380653478723118886195e9dad481b903195394", "4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f", "72f32dec60e352e652006b0a09ed8720b4c88e4afc177ecde22266a9803d7203"),
	"116_auth_identity_legacy_external_safety_reports.sql":    newMigrationChecksumCompatibilityRule("07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488", "aee893b8afb6bbbdb37fc4ecc320438ad6f79b2f5a395e27d45fd6d82a9e0df6", "f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877", "a4db306b0b987459590522ebb08ff9ce42ab1ff5d4f99ec4068c41a51f2236da"),
	"118_wechat_dual_mode_and_auth_source_defaults.sql":       newMigrationChecksumCompatibilityRule("ed272e0840730b6b8e7838513c4cc8817e8b5e488e27c88b5421adbece5e89c9", "b4a5b7a28f6a7ac67aad214645761e5a8486c83f0f2a1a874d7f67085f83159b", "6395ad255f2be2219ad85813b72db6fa7783c81d747e42e098847ef3594f1674"),
	"119_enforce_payment_orders_out_trade_no_unique.sql":      newMigrationChecksumCompatibilityRule("0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e", "ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34"),
	"120_enforce_payment_orders_out_trade_no_unique_notx.sql": newMigrationChecksumCompatibilityRule("34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074", "e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61", "79ea6127a22e61b3bad6ea29347a8cc3ff005f8b486ef4a51bd04fdda906f931"),
	"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": newMigrationChecksumCompatibilityRule("7faba5ef65051b7ecb215b7fd2351b0828b7c48153ec688ac089c1588d2cde41", "ac0d79ca6feb449674f54f593a5eac5f7cc06751047c664b586c1892e19c60d5", "ea17c2767b937f08274e091d212a93acb7e2d62521129179830f073a291fbd97"),
	"159_batch_image_foundation.sql":                          newMigrationChecksumCompatibilityRule("d902b70982025ec519749faf058aab7631e82c3f48167b9a4ae4db718eb72cce", "82da85b5d98e67a0507647b873a40373e84538e4adafdeed6767c0ac8b6570b2"),
	"161_batch_image_pricing_snapshot.sql":                    newMigrationChecksumCompatibilityRule("4012af3e43636cb6af22e0176d59d1fcc70615c0f310194329461ae462c4fbd6", "96d915c9b7a6941ae99039e0ff3f1a61481eb9bddd933d11c6fadb2274554e87"),
	// 195 originally seeded mode=v2; flipped to v1 (safe default / opt-in v2). Existing DBs
	// that already applied the v2 seed keep their row and the historical checksum.
	"195_channel_monitor_mode.sql": newMigrationChecksumCompatibilityRule("73c39ac374c722253135041466108836845828a6065b499c60e7f27d6b92c21c", "f20366e106e3a54c73d4a67df3ba87734427ed859bc4ae42b0708e4cbcbacb56"),
	// 220 originally cleared video prices for all non-grok platforms (including composite);
	// composite is now preserved because it may route to Grok accounts.
	"220_clear_non_grok_video_generation_config.sql": newMigrationChecksumCompatibilityRule("cf4dbfa75ac27d93a30a6a14439fe7dccfc911c043358363d5ec47946aa0e28b", "353c8e8e1805f2a6fd61311e03118e7dd8388f264cfd9af9e0cabe2a696388c4", "3d08d905a7bca1f56f14b6d2a2a0dcb07480ff52c21393b4e2db1b3a3f83b3d0"),
	"219_group_search_price_per_1k.sql":              newMigrationChecksumCompatibilityRule("430c2e3595342fe22c59e9676e9b18ea376f076324b77174a21e6f181f57f4b5", "833578274d0eed24d39355298d5659b33e5484c869b331ffd815187c221552d2"),
	"218_group_audio_voice_pricing.sql":              newMigrationChecksumCompatibilityRule("a99ade7d0d464c67bf56814570050cc363ffad64eae2cb1e1ed760065f0b3585", "343a955e52348ce92c35753e78ca3f8e5a76060c20af71061ca5e04c6ed84085"),
}

// ApplyMigrations 将嵌入的 SQL 迁移文件应用到指定的数据库。
//
// 该函数可以在每次应用启动时安全调用：
// - 已应用的迁移会被自动跳过（通过校验 filename 判断）
// - 如果迁移文件内容被修改（checksum 不匹配），会返回错误
// - 使用 PostgreSQL Advisory Lock 确保多实例并发安全
//
// 参数：
//   - ctx: 上下文，用于超时控制和取消
//   - db: 数据库连接
//
// 返回：
//   - error: 迁移过程中的任何错误
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	return applyMigrationsFS(ctx, db, migrations.FS)
}

// ApplyMigrationsForSchema applies canonical migrations while adapting explicit
// upstream "public" references when DATABASE_SCHEMA is configured. The database
// connection itself must already use the matching schema-first search_path.
func ApplyMigrationsForSchema(ctx context.Context, db *sql.DB, schema string) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	schema = config.EffectivePostgresSchema(schema)
	return applyMigrationsFSForSchema(ctx, db, migrations.FS, schema)
}

// applyMigrationsFS 是迁移执行的核心实现。
// 它从指定的文件系统读取 SQL 迁移文件并按顺序应用。
//
// 迁移执行流程：
//  1. 获取 PostgreSQL Advisory Lock，防止多实例并发迁移
//  2. 确保 schema_migrations 表存在
//  3. 按文件名排序读取所有 .sql 文件
//  4. 对于每个迁移文件：
//     - 计算文件内容的 SHA256 校验和
//     - 检查该迁移是否已应用（通过 filename 查询）
//     - 如果已应用，验证校验和是否匹配
//     - 如果未应用，在事务中执行迁移并记录
//  5. 释放 Advisory Lock
//
// 参数：
//   - ctx: 上下文
//   - db: 数据库连接
//   - fsys: 包含迁移文件的文件系统（通常是 embed.FS）
func applyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	return applyMigrationsFSForSchema(ctx, db, fsys, "")
}

func applyMigrationsFSForSchema(ctx context.Context, db *sql.DB, fsys fs.FS, schema string) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	schema = config.EffectivePostgresSchema(schema)
	if !config.IsValidPostgresIdentifier(schema) {
		return fmt.Errorf("invalid database schema %q; use lowercase letters, digits, and underscores", schema)
	}

	// 获取分布式锁，确保多实例部署时只有一个实例执行迁移。
	// 这是 PostgreSQL 特有的 Advisory Lock 机制。
	lockConn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migrations lock connection: %w", err)
	}
	defer func() { _ = lockConn.Close() }()
	if err := pgAdvisoryLock(ctx, lockConn); err != nil {
		return err
	}
	defer func() {
		// 无论迁移是否成功，都要释放锁。
		// 独立超时确保原 ctx 取消后仍会尝试释放，但数据库链路异常不会
		// 无限阻塞进程退出。
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pgAdvisoryUnlock(unlockCtx, lockConn)
	}()

	var currentSchema sql.NullString
	if err := lockConn.QueryRowContext(ctx, "SELECT current_schema()").Scan(&currentSchema); err != nil {
		return fmt.Errorf("check current database schema: %w", err)
	}
	if !currentSchema.Valid || currentSchema.String != schema {
		return databaseSchemaMismatchError(schema, currentSchema.String)
	}

	// 创建迁移记录表（如果不存在）。
	// 该表记录所有已应用的迁移及其校验和。
	if _, err := lockConn.ExecContext(ctx, schemaMigrationsTableDDL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 自动对齐 Atlas 基线（如果检测到 legacy schema_migrations 且缺失 atlas_schema_revisions）。
	if err := ensureAtlasBaselineAligned(ctx, lockConn, fsys); err != nil {
		return err
	}

	// 获取所有 .sql 迁移文件并按文件名排序。
	// 命名规范：使用零填充数字前缀（如 001_init.sql, 002_add_users.sql）。
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files) // 确保按文件名顺序执行迁移

	for _, name := range files {
		// 读取迁移文件内容
		contentBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue // 跳过空文件
		}

		// 计算文件内容的 SHA256 校验和，用于检测文件是否被修改。
		// 这是一种防篡改机制：如果有人修改了已应用的迁移文件，系统会拒绝启动。
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])
		executionContent, err := migrationSQLForSchema(name, content, schema)
		if err != nil {
			return fmt.Errorf("adapt migration %s for schema %q: %w", name, schema, err)
		}

		// 检查该迁移是否已经应用
		var existing string
		rowErr := lockConn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE filename = $1", name).Scan(&existing)
		if rowErr == nil {
			// 迁移已应用，验证校验和是否匹配
			if existing != checksum {
				// 兼容特定历史误改场景（仅白名单规则），其余仍保持严格不可变约束。
				if isMigrationChecksumCompatible(name, existing, checksum) {
					continue
				}
				// 校验和不匹配意味着迁移文件在应用后被修改，这是危险的。
				// 正确的做法是创建新的迁移文件来进行变更。
				return fmt.Errorf(
					"migration %s checksum mismatch (db=%s file=%s)\n"+
						"This means the migration file was modified after being applied to the database.\n"+
						"Solutions:\n"+
						"  1. Revert to original: git log --oneline -- migrations/%s && git checkout <commit> -- migrations/%s\n"+
						"  2. For new changes, create a new migration file instead of modifying existing ones\n"+
						"Note: Modifying applied migrations breaks the immutability principle and can cause inconsistencies across environments",
					name, existing, checksum, name, name,
				)
			}
			continue // 迁移已应用且校验和匹配，跳过
		}
		if !errors.Is(rowErr, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", name, rowErr)
		}

		nonTx, err := validateMigrationExecutionMode(name, executionContent)
		if err != nil {
			return fmt.Errorf("validate migration %s: %w", name, err)
		}

		if nonTx {
			if err := prepareNonTransactionalMigration(ctx, lockConn, name); err != nil {
				return fmt.Errorf("prepare migration %s: %w", name, err)
			}

			// *_notx.sql：用于 CREATE/DROP INDEX CONCURRENTLY 场景，必须非事务执行。
			// 逐条语句执行，避免将多条 CONCURRENTLY 语句放入同一个隐式事务块。
			statements := splitSQLStatements(executionContent)
			for i, stmt := range statements {
				trimmed := strings.TrimSpace(stmt)
				if trimmed == "" {
					continue
				}
				if stripSQLLineComment(trimmed) == "" {
					continue
				}
				if _, err := lockConn.ExecContext(ctx, trimmed); err != nil {
					return fmt.Errorf("apply migration %s (non-tx statement %d): %w", name, i+1, err)
				}
			}
			if _, err := lockConn.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
				return fmt.Errorf("record migration %s (non-tx): %w", name, err)
			}
			continue
		}

		// 默认迁移在事务中执行，确保原子性：要么完全成功，要么完全回滚。
		tx, err := lockConn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}

		// 执行迁移 SQL
		if _, err := tx.ExecContext(ctx, executionContent); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		// 记录迁移已完成，保存文件名和校验和
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		// 提交事务
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}

type migrationSchemaRewriteRule struct {
	publicQualifierCount int
	tableSchemaCount     int
	schemaNameCount      int
}

// migrationSchemaRewriteRules is deliberately narrow. Published migrations stay
// byte-for-byte canonical for checksums; only these known upstream references to
// the default schema are adapted in the execution copy.
var migrationSchemaRewriteRules = map[string]migrationSchemaRewriteRule{
	"006_add_users_allowed_groups_compat.sql":               {publicQualifierCount: 1, tableSchemaCount: 1},
	"006_fix_invalid_subscription_expires_at.sql":           {publicQualifierCount: 1},
	"006b_guard_users_allowed_groups.sql":                   {tableSchemaCount: 2},
	"009_fix_usage_logs_cache_columns.sql":                  {tableSchemaCount: 2},
	"108a_widen_auth_identity_migration_report_type.sql":    {tableSchemaCount: 1},
	"115_auth_identity_legacy_external_backfill.sql":        {publicQualifierCount: 7},
	"116_auth_identity_legacy_external_safety_reports.sql":  {publicQualifierCount: 20},
	"120a_align_payment_orders_out_trade_no_index_name.sql": {schemaNameCount: 2},
	"175_default_openai_long_context_billing.sql":           {publicQualifierCount: 4},
}

type migrationExecutionReplacement struct {
	from  string
	to    string
	count int
}

// migrationSchemaIsolationRewrites fixes canonical migrations whose catalog
// checks identify objects only by name. PostgreSQL constraint and relation names
// are schema-local, while pg_catalog and information_schema queries span the
// whole database; without these predicates, a same-named object in another
// schema can make a migration skip required DDL or return multiple rows.
//
// The canonical embedded files remain byte-for-byte unchanged for checksum
// compatibility. Every replacement is count-pinned so an upstream SQL change
// fails closed and must be reviewed before execution.
var migrationSchemaIsolationRewrites = map[string][]migrationExecutionReplacement{
	"035_usage_logs_partitioning.sql": {
		{
			from:  "WHERE c.relname = 'usage_logs'",
			to:    "WHERE c.oid = 'usage_logs'::regclass",
			count: 1,
		},
	},
	"061_add_usage_log_request_type.sql": {
		{
			from:  "WHERE conname = 'usage_logs_request_type_check'",
			to:    "WHERE conname = 'usage_logs_request_type_check'\n          AND conrelid = 'usage_logs'::regclass",
			count: 1,
		},
	},
	"108_auth_identity_foundation_core.sql": {
		{
			from:  "WHERE conname = 'users_signup_source_check'",
			to:    "WHERE conname = 'users_signup_source_check'\n          AND conrelid = 'users'::regclass",
			count: 1,
		},
	},
	"116_auth_identity_legacy_external_safety_reports.sql": {
		{
			from:  "WHERE conname = 'auth_identities_metadata_is_object_check'",
			to:    "WHERE conname = 'auth_identities_metadata_is_object_check'\n          AND conrelid = 'auth_identities'::regclass",
			count: 1,
		},
		{
			from:  "WHERE conname = 'auth_identity_channels_metadata_is_object_check'",
			to:    "WHERE conname = 'auth_identity_channels_metadata_is_object_check'\n          AND conrelid = 'auth_identity_channels'::regclass",
			count: 1,
		},
		{
			from:  "WHERE conname = 'auth_identity_migration_reports_details_is_object_check'",
			to:    "WHERE conname = 'auth_identity_migration_reports_details_is_object_check'\n          AND conrelid = 'auth_identity_migration_reports'::regclass",
			count: 1,
		},
	},
	"128_add_channel_monitor_request_templates.sql": {
		{
			from:  "AND table_name = 'channel_monitors'",
			to:    "AND table_name = 'channel_monitors'\n          AND table_schema = current_schema()",
			count: 2,
		},
	},
	"138_channel_monitor_openai_api_mode.sql": {
		{
			from:  "AND table_name = 'channel_monitors'",
			to:    "AND table_name = 'channel_monitors'\n          AND table_schema = current_schema()",
			count: 1,
		},
		{
			from:  "AND table_name = 'channel_monitor_request_templates'",
			to:    "AND table_name = 'channel_monitor_request_templates'\n          AND table_schema = current_schema()",
			count: 1,
		},
	},
	"154_account_spark_shadow.sql": {
		{
			from:  "WHERE conname = 'chk_accounts_quota_dimension'",
			to:    "WHERE conname = 'chk_accounts_quota_dimension' AND conrelid = 'accounts'::regclass",
			count: 1,
		},
		{
			from:  "WHERE conname = 'chk_accounts_parent_dimension'",
			to:    "WHERE conname = 'chk_accounts_parent_dimension' AND conrelid = 'accounts'::regclass",
			count: 1,
		},
		{
			from:  "WHERE conname = 'chk_accounts_parent_not_self'",
			to:    "WHERE conname = 'chk_accounts_parent_not_self' AND conrelid = 'accounts'::regclass",
			count: 1,
		},
		{
			from:  "WHERE conname = 'fk_accounts_parent_account_id'",
			to:    "WHERE conname = 'fk_accounts_parent_account_id' AND conrelid = 'accounts'::regclass",
			count: 1,
		},
	},
	"176_channel_monitor_grok_provider.sql": {
		{
			from:  "WHERE t.relname = 'channel_monitors'",
			to:    "WHERE t.oid = 'channel_monitors'::regclass",
			count: 1,
		},
		{
			from:  "WHERE t.relname = 'channel_monitor_request_templates'",
			to:    "WHERE t.oid = 'channel_monitor_request_templates'::regclass",
			count: 1,
		},
	},
}

const (
	publicQualifierPattern  = "public."
	publicTableSchemaFilter = "table_schema = 'public'"
	publicSchemaNameFilter  = "schemaname = 'public'"
)

// migrationSQLForSchema keeps embedded migration files canonical while
// adapting a verified execution copy for DATABASE_SCHEMA. Any count drift or a
// new migration with an explicit public-schema reference fails closed so an
// upstream SQL change cannot be silently rewritten incorrectly.
func migrationSQLForSchema(name, canonical, schema string) (string, error) {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "public"
	}
	if !config.IsValidPostgresIdentifier(schema) {
		return "", fmt.Errorf("invalid database schema %q; use lowercase letters, digits, and underscores", schema)
	}

	executionSQL := canonical
	if schema != "public" {
		rule, allowed := migrationSchemaRewriteRules[name]
		if !allowed {
			if hasSchemaSensitivePublicReference(canonical) {
				return "", errors.New("contains an unreviewed explicit public-schema reference")
			}
		} else {
			actualPublicQualifiers := strings.Count(canonical, publicQualifierPattern)
			actualTableSchemaFilters := strings.Count(canonical, publicTableSchemaFilter)
			actualSchemaNameFilters := strings.Count(canonical, publicSchemaNameFilter)
			if actualPublicQualifiers != rule.publicQualifierCount ||
				actualTableSchemaFilters != rule.tableSchemaCount ||
				actualSchemaNameFilters != rule.schemaNameCount {
				return "", fmt.Errorf(
					"schema rewrite expectation drifted (public qualifiers=%d/%d, table_schema filters=%d/%d, schemaname filters=%d/%d)",
					actualPublicQualifiers,
					rule.publicQualifierCount,
					actualTableSchemaFilters,
					rule.tableSchemaCount,
					actualSchemaNameFilters,
					rule.schemaNameCount,
				)
			}

			quotedSchema := quotePostgresIdentifier(schema)
			executionSQL = strings.ReplaceAll(executionSQL, publicTableSchemaFilter, "table_schema = '"+schema+"'")
			executionSQL = strings.ReplaceAll(executionSQL, publicSchemaNameFilter, "schemaname = '"+schema+"'")
			executionSQL = strings.ReplaceAll(executionSQL, publicQualifierPattern, quotedSchema+".")
		}
	}

	if replacements := migrationSchemaIsolationRewrites[name]; len(replacements) > 0 {
		var err error
		executionSQL, err = applyMigrationExecutionReplacements(name, executionSQL, replacements)
		if err != nil {
			return "", err
		}
	}

	if schema != "public" && hasSchemaSensitivePublicReference(executionSQL) {
		return "", errors.New("explicit public-schema reference remained after adaptation")
	}
	return executionSQL, nil
}

func applyMigrationExecutionReplacements(
	name string,
	content string,
	replacements []migrationExecutionReplacement,
) (string, error) {
	executionSQL := content
	for index, replacement := range replacements {
		actual := strings.Count(executionSQL, replacement.from)
		if actual != replacement.count {
			return "", fmt.Errorf(
				"schema isolation rewrite expectation drifted for %s rule %d (matches=%d/%d)",
				name,
				index+1,
				actual,
				replacement.count,
			)
		}
		executionSQL = strings.ReplaceAll(executionSQL, replacement.from, replacement.to)
	}
	return executionSQL, nil
}

// hasSchemaSensitivePublicReference recognizes the forms used by migrations
// without treating ordinary string data (for example URLs containing
// "public.example") as a schema reference.
func hasSchemaSensitivePublicReference(content string) bool {
	lower := strings.ToLower(content)
	if containsPublicCatalogFilter(lower) || containsPublicRegclassReference(lower) {
		return true
	}
	return containsPublicQualifierOutsideStringOrComment(lower)
}

func containsPublicCatalogFilter(content string) bool {
	for _, column := range []string{"table_schema", "schemaname"} {
		searchFrom := 0
		for {
			offset := strings.Index(content[searchFrom:], column)
			if offset < 0 {
				break
			}
			offset += searchFrom + len(column)
			remainder := strings.TrimLeft(content[offset:], " \t\r\n")
			if strings.HasPrefix(remainder, "=") {
				remainder = strings.TrimLeft(remainder[1:], " \t\r\n")
				if strings.HasPrefix(remainder, "'public'") {
					return true
				}
			}
			searchFrom = offset
		}
	}
	return false
}

func containsPublicRegclassReference(content string) bool {
	for _, functionName := range []string{"to_regclass", "to_regnamespace"} {
		searchFrom := 0
		for {
			offset := strings.Index(content[searchFrom:], functionName)
			if offset < 0 {
				break
			}
			offset += searchFrom + len(functionName)
			remainder := strings.TrimLeft(content[offset:], " \t\r\n")
			if strings.HasPrefix(remainder, "(") {
				remainder = strings.TrimLeft(remainder[1:], " \t\r\n")
				if strings.HasPrefix(remainder, "'public.") || strings.HasPrefix(remainder, "'\"public\".") {
					return true
				}
			}
			searchFrom = offset
		}
	}
	return false
}

func containsPublicQualifierOutsideStringOrComment(content string) bool {
	masked := maskSQLSingleQuotedStringsAndComments(content)
	for _, qualifier := range []string{"public.", `"public".`} {
		searchFrom := 0
		for {
			offset := strings.Index(masked[searchFrom:], qualifier)
			if offset < 0 {
				break
			}
			offset += searchFrom
			if offset == 0 || !isPostgresIdentifierPart(masked[offset-1]) {
				return true
			}
			searchFrom = offset + len(qualifier)
		}
	}
	return false
}

func maskSQLSingleQuotedStringsAndComments(content string) string {
	masked := []byte(content)
	const (
		sqlNormal = iota
		sqlSingleQuoted
		sqlLineComment
		sqlBlockComment
	)
	state := sqlNormal
	for i := 0; i < len(masked); i++ {
		switch state {
		case sqlNormal:
			switch {
			case masked[i] == '\'':
				masked[i] = ' '
				state = sqlSingleQuoted
			case masked[i] == '-' && i+1 < len(masked) && masked[i+1] == '-':
				masked[i], masked[i+1] = ' ', ' '
				i++
				state = sqlLineComment
			case masked[i] == '/' && i+1 < len(masked) && masked[i+1] == '*':
				masked[i], masked[i+1] = ' ', ' '
				i++
				state = sqlBlockComment
			}
		case sqlSingleQuoted:
			if masked[i] == '\'' {
				masked[i] = ' '
				if i+1 < len(masked) && masked[i+1] == '\'' {
					masked[i+1] = ' '
					i++
				} else {
					state = sqlNormal
				}
			} else {
				masked[i] = ' '
			}
		case sqlLineComment:
			if masked[i] == '\n' {
				state = sqlNormal
			} else {
				masked[i] = ' '
			}
		case sqlBlockComment:
			if masked[i] == '*' && i+1 < len(masked) && masked[i+1] == '/' {
				masked[i], masked[i+1] = ' ', ' '
				i++
				state = sqlNormal
			} else {
				masked[i] = ' '
			}
		}
	}
	return string(masked)
}

func isPostgresIdentifierPart(value byte) bool {
	return value == '_' || value == '$' ||
		(value >= 'a' && value <= 'z') ||
		(value >= '0' && value <= '9')
}

type migrationConnection interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func prepareNonTransactionalMigration(ctx context.Context, db migrationConnection, name string) error {
	switch name {
	case paymentOrdersOutTradeNoUniqueMigration:
		return preparePaymentOrdersOutTradeNoUniqueMigration(ctx, db)
	case schedulerOutboxPendingDedupKeyMigration:
		return dropInvalidIndexIfPresent(ctx, db, schedulerOutboxPendingDedupKeyIndex)
	case accountSparkShadowIndexesMigration:
		for _, indexName := range []string{accountParentAccountIDIndex, accountSparkShadowPerParentIndex} {
			if err := dropInvalidIndexIfPresent(ctx, db, indexName); err != nil {
				return err
			}
		}
		return nil
	case latestAPIKeyIPIndexMigration:
		return dropInvalidIndexIfPresent(ctx, db, latestAPIKeyIPIndex)
	case usageLogsUpstreamModelMismatchIndexMigration:
		return dropInvalidIndexIfPresent(ctx, db, usageLogsUpstreamModelMismatchIndex)
	default:
		return nil
	}
}

func preparePaymentOrdersOutTradeNoUniqueMigration(ctx context.Context, db migrationConnection) error {
	duplicates, err := findDuplicatePaymentOrderOutTradeNos(ctx, db)
	if err != nil {
		return fmt.Errorf("precheck duplicate out_trade_no: %w", err)
	}
	if len(duplicates) > 0 {
		return fmt.Errorf(
			"duplicate out_trade_no values block %s; remediate duplicates before retrying: %s",
			paymentOrdersOutTradeNoUniqueMigration,
			strings.Join(duplicates, ", "),
		)
	}

	return dropInvalidIndexIfPresent(ctx, db, paymentOrdersOutTradeNoUniqueIndex)
}

func dropInvalidIndexIfPresent(ctx context.Context, db migrationConnection, indexName string) error {
	invalid, err := indexIsInvalid(ctx, db, indexName)
	if err != nil {
		return fmt.Errorf("check invalid index %s: %w", indexName, err)
	}
	if !invalid {
		return nil
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s", indexName)); err != nil {
		return fmt.Errorf("drop invalid index %s: %w", indexName, err)
	}
	return nil
}

func findDuplicatePaymentOrderOutTradeNos(ctx context.Context, db migrationConnection) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT out_trade_no, COUNT(*) AS duplicate_count
		FROM payment_orders
		WHERE out_trade_no <> ''
		GROUP BY out_trade_no
		HAVING COUNT(*) > 1
		ORDER BY duplicate_count DESC, out_trade_no
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	duplicates := make([]string, 0, 5)
	for rows.Next() {
		var outTradeNo string
		var duplicateCount int
		if err := rows.Scan(&outTradeNo, &duplicateCount); err != nil {
			return nil, err
		}
		duplicates = append(duplicates, fmt.Sprintf("%s (count=%d)", outTradeNo, duplicateCount))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return duplicates, nil
}

func indexIsInvalid(ctx context.Context, db migrationConnection, indexName string) (bool, error) {
	var invalid bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_class idx
			JOIN pg_namespace ns ON ns.oid = idx.relnamespace
			JOIN pg_index i ON i.indexrelid = idx.oid
			WHERE ns.nspname = current_schema()
			  AND idx.relname = $1
			  AND NOT i.indisvalid
		)
	`, indexName).Scan(&invalid)
	return invalid, err
}

func ensureAtlasBaselineAligned(ctx context.Context, db migrationConnection, fsys fs.FS) error {
	hasLegacy, err := tableExists(ctx, db, "schema_migrations")
	if err != nil {
		return fmt.Errorf("check schema_migrations: %w", err)
	}
	if !hasLegacy {
		return nil
	}

	hasAtlas, err := tableExists(ctx, db, "atlas_schema_revisions")
	if err != nil {
		return fmt.Errorf("check atlas_schema_revisions: %w", err)
	}
	if !hasAtlas {
		if _, err := db.ExecContext(ctx, atlasSchemaRevisionsTableDDL); err != nil {
			return fmt.Errorf("create atlas_schema_revisions: %w", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM atlas_schema_revisions").Scan(&count); err != nil {
		return fmt.Errorf("count atlas_schema_revisions: %w", err)
	}
	if count > 0 {
		return nil
	}

	version, description, hash, err := latestMigrationBaseline(fsys)
	if err != nil {
		return fmt.Errorf("atlas baseline version: %w", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO atlas_schema_revisions (version, description, type, applied, total, executed_at, execution_time, hash)
		VALUES ($1, $2, $3, 0, 0, NOW(), 0, $4)
	`, version, description, 1, hash); err != nil {
		return fmt.Errorf("insert atlas baseline: %w", err)
	}
	return nil
}

func tableExists(ctx context.Context, db migrationConnection, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = $1
		)
	`, tableName).Scan(&exists)
	return exists, err
}

func latestMigrationBaseline(fsys fs.FS) (string, string, string, error) {
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return "", "", "", err
	}
	if len(files) == 0 {
		return "baseline", "baseline", "", nil
	}
	sort.Strings(files)
	name := files[len(files)-1]
	contentBytes, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", "", "", err
	}
	content := strings.TrimSpace(string(contentBytes))
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	version := strings.TrimSuffix(name, ".sql")
	return version, version, hash, nil
}

func checksumSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func newMigrationChecksumCompatibilityRule(fileChecksum string, acceptedDBChecksums ...string) migrationChecksumCompatibilityRule {
	return migrationChecksumCompatibilityRule{
		fileChecksum:       fileChecksum,
		acceptedDBChecksum: checksumSet(acceptedDBChecksums...),
	}
}

func isMigrationChecksumCompatible(name, dbChecksum, fileChecksum string) bool {
	rule, ok := migrationChecksumCompatibilityRules[name]
	if !ok {
		return false
	}
	if fileChecksum != rule.fileChecksum {
		return false
	}
	if dbChecksum == rule.fileChecksum {
		return true
	}
	_, dbOK := rule.acceptedDBChecksum[dbChecksum]
	return dbOK
}

func validateMigrationExecutionMode(name, content string) (bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	upperContent := strings.ToUpper(content)
	nonTx := strings.HasSuffix(normalizedName, nonTransactionalMigrationSuffix)

	if !nonTx {
		if strings.Contains(upperContent, "CONCURRENTLY") {
			return false, errors.New("CONCURRENTLY statements must be placed in *_notx.sql migrations")
		}
		return false, nil
	}

	if strings.Contains(upperContent, "BEGIN") || strings.Contains(upperContent, "COMMIT") || strings.Contains(upperContent, "ROLLBACK") {
		return false, errors.New("*_notx.sql must not contain transaction control statements (BEGIN/COMMIT/ROLLBACK)")
	}

	statements := splitSQLStatements(content)
	for _, stmt := range statements {
		normalizedStmt := strings.ToUpper(stripSQLLineComment(strings.TrimSpace(stmt)))
		if normalizedStmt == "" {
			continue
		}

		if strings.Contains(normalizedStmt, "CONCURRENTLY") {
			isCreateIndex := strings.Contains(normalizedStmt, "CREATE") && strings.Contains(normalizedStmt, "INDEX")
			isDropIndex := strings.Contains(normalizedStmt, "DROP") && strings.Contains(normalizedStmt, "INDEX")
			if !isCreateIndex && !isDropIndex {
				return false, errors.New("*_notx.sql currently only supports CREATE/DROP INDEX CONCURRENTLY statements")
			}
			if isCreateIndex && !strings.Contains(normalizedStmt, "IF NOT EXISTS") {
				return false, errors.New("CREATE INDEX CONCURRENTLY in *_notx.sql must include IF NOT EXISTS for idempotency")
			}
			if isDropIndex && !strings.Contains(normalizedStmt, "IF EXISTS") {
				return false, errors.New("DROP INDEX CONCURRENTLY in *_notx.sql must include IF EXISTS for idempotency")
			}
			continue
		}

		return false, errors.New("*_notx.sql must not mix non-CONCURRENTLY SQL statements")
	}

	return true, nil
}

func splitSQLStatements(content string) []string {
	parts := strings.Split(content, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func stripSQLLineComment(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// pgAdvisoryLock 获取 PostgreSQL Advisory Lock。
// Advisory Lock 是一种轻量级的锁机制，不与任何特定的数据库对象关联。
// 它非常适合用于应用层面的分布式锁场景，如迁移序列化。
type advisoryLockConnection interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func pgAdvisoryLock(ctx context.Context, db advisoryLockConnection) error {
	ticker := time.NewTicker(migrationsLockRetryInterval)
	defer ticker.Stop()

	for {
		var locked bool
		if err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrationsAdvisoryLockID).Scan(&locked); err != nil {
			return fmt.Errorf("acquire migrations lock: %w", err)
		}
		if locked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire migrations lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// pgAdvisoryUnlock 释放 PostgreSQL Advisory Lock。
// 必须在获取锁后确保释放，否则会阻塞其他实例的迁移操作。
func pgAdvisoryUnlock(ctx context.Context, db advisoryLockConnection) error {
	_, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationsAdvisoryLockID)
	if err != nil {
		return fmt.Errorf("release migrations lock: %w", err)
	}
	return nil
}
