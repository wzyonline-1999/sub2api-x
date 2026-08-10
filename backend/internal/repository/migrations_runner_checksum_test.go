package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigrationChecksumCompatibilityRulesMatchCanonicalFiles(t *testing.T) {
	for name, rule := range migrationChecksumCompatibilityRules {
		content, err := migrations.FS.ReadFile(name)
		require.NoError(t, err, "compatibility rule references a missing migration: %s", name)

		sum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))
		require.Equal(
			t,
			rule.fileChecksum,
			hex.EncodeToString(sum[:]),
			"compatibility rule must pin the current canonical migration: %s",
			name,
		)
	}
}

func TestMigrationChecksumCompatibilityRulesRejectRawFileHashes(t *testing.T) {
	rawHashes := map[string][]string{
		"109_auth_identity_compat_backfill.sql": {
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
		},
		"110_pending_auth_and_provider_default_grants.sql": {
			"32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279",
			"e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925",
		},
		"112_add_payment_order_provider_key_snapshot.sql": {
			"b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99",
			"ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e",
		},
		"118_wechat_dual_mode_and_auth_source_defaults.sql": {
			"b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0",
			"e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227",
			"a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb",
		},
		"120_enforce_payment_orders_out_trade_no_unique_notx.sql": {
			"707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22",
			"04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a",
		},
		"123_fix_legacy_auth_source_grant_on_signup_defaults.sql": {
			"2ce43c2cd89e9f9e1febd34a407ed9e84d177386c5544b6f02c1f58a21129f57",
			"6cd33422f215dcd1f486ab6f35c0ea5805d9ca69bb25906d94bc649156657145",
		},
		"195_channel_monitor_mode.sql": {
			"13f3792f3e3e53ee96e26415c884cf8062c77172824b54fcc9a8c0c2b1f185ec",
			"4c74fe33ef2274cc72e1bb49671e651274532c034b29f5b2982c2a4c88d101a6",
		},
		"218_group_audio_voice_pricing.sql": {
			"40ee9f3a2af0e0a5e99dabc878fd0fe98be1011f26bcfcefcac7197f7081f0e7",
			"c2a5e5b4ffd6968ad1c10593289fbc11192cdea19fec3ed9bce3a84eff9a8351",
		},
		"219_group_search_price_per_1k.sql": {
			"e86786ebcc3b14206fd2d321380a4e50e80cdadbfcf4962c639255e6a14008db",
			"df6ffd71b97e30ec2c8fe7b95e15783042dea58c553e32701ee7c42a5619af80",
		},
		"220_clear_non_grok_video_generation_config.sql": {
			"85e320b9ec64f2d3fcd8cf705b2b4e76a7b49f7a57140c14bff97f32691c818b",
			"3da48c8fdffe6390325f43d08b8e353e0a365df43d44a78dbbe655d0deb18402",
			"e7942a7201f9a0d35e78275fbbe4eca82ac25e4a3741920e45bcd1054e0522a8",
		},
	}

	for name, hashes := range rawHashes {
		rule := migrationChecksumCompatibilityRules[name]
		for _, hash := range hashes {
			require.Falsef(
				t,
				isMigrationChecksumCompatible(name, hash, rule.fileChecksum),
				"%s must accept only the TrimSpace checksum actually stored by the runner",
				name,
			)
		}
	}
}

func TestCanonicalMigrationsSupportConfiguredSchemaAtExecution(t *testing.T) {
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)

	for _, name := range names {
		content, readErr := migrations.FS.ReadFile(name)
		require.NoError(t, readErr)
		executionSQL, rewriteErr := migrationSQLForSchema(name, string(content), "tenant_a")
		require.NoError(t, rewriteErr, name)
		require.False(t, hasSchemaSensitivePublicReference(executionSQL), name)
	}
}

func TestMigrationSQLForSchemaLeavesDefaultSchemaByteIdentical(t *testing.T) {
	canonical := " SELECT * FROM public.users;\n"
	for _, schema := range []string{"", "public", " public "} {
		executionSQL, err := migrationSQLForSchema("unknown.sql", canonical, schema)
		require.NoError(t, err)
		require.Equal(t, canonical, executionSQL)
	}
}

func TestIsMigrationChecksumCompatible(t *testing.T) {
	t.Run("054历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.True(t, ok)
	})

	t.Run("054在未知文件checksum下不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"0000000000000000000000000000000000000000000000000000000000000000",
		)
		require.False(t, ok)
	})

	t.Run("061历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("061第二个历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("非白名单迁移不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"001_init.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.False(t, ok)
	})

	t.Run("恢复官方006系列文件后仍兼容二开数据库checksum", func(t *testing.T) {
		cases := []struct {
			name         string
			dbChecksum   string
			fileChecksum string
		}{
			{
				name:         "006_add_users_allowed_groups_compat.sql",
				dbChecksum:   "c1301d7c0d9cb7a25e7a00ddc930992a6b645270d4379fd6dac023f847861169",
				fileChecksum: "900f5ba934e8d66bba7d94f1d34463a9022e0e72c3ce911260d7c703449a33ae",
			},
			{
				name:         "006_fix_invalid_subscription_expires_at.sql",
				dbChecksum:   "4ca60e82e381d97f19f4a6f1046a77c7d147a727553f0aea9417c3674267c0d3",
				fileChecksum: "ed5d6553a86578d987088331bc8810808b75e554a160b66ff4626815ea838354",
			},
			{
				name:         "006b_guard_users_allowed_groups.sql",
				dbChecksum:   "cb97c4944338921bd9cdbba5eee7071abd775f67456798716637e0e1575b0de6",
				fileChecksum: "6953b71b92d0ed2f035137fdc4e4fb6cc056861b22e28c5612950c08cedf564e",
			},
			{
				name:         "009_fix_usage_logs_cache_columns.sql",
				dbChecksum:   "e2127eb45db1fdd6c717aa77227831e542457be95bff8f7f28669197853b060d",
				fileChecksum: "9a3c22b296ea5a628eb8b35b34318f035b9ac177dfc7d1db26b0901dcd98bafb",
			},
		}
		for _, tc := range cases {
			ok := isMigrationChecksumCompatible(tc.name, tc.dbChecksum, tc.fileChecksum)
			require.True(t, ok)
		}
	})

	t.Run("恢复官方108a文件后仍兼容二开数据库checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"108a_widen_auth_identity_migration_report_type.sql",
			"ce0461cef9871b3042eefebc6065c94a21d61c339aa82eb44d95c19e5f4d9062",
			"87646a866b17a329b01abb241b99e3874ee2c13c7d7bba3f6e37c53722a60e18",
		)
		require.True(t, ok)
	})

	t.Run("109历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"748ddcdc60f93a1ac562ce8a66ee870f64ee594bf6dbedad55ed8baf3c75b28c",
			"2b380305e73ff0c13aa8c811e45897f2b36ca4a438f7b3e8f98e19ecb6bae0b3",
		)
		require.True(t, ok)
	})

	t.Run("109不接受未规范化的原始文件checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"2b380305e73ff0c13aa8c811e45897f2b36ca4a438f7b3e8f98e19ecb6bae0b3",
		)
		require.False(t, ok)
	})

	t.Run("109拒绝把历史文件重新作为当前文件", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
		)
		require.False(t, ok)
	})

	t.Run("110历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"110_pending_auth_and_provider_default_grants.sql",
			"301e90405b3424967b7d1931568b7a244902148fa82802f362c115ae4e2ae2ef",
			"57a196a9810fb478fa001dfff110f5c76a7d87fb04f15e12e513fcb75402d7a6",
		)
		require.True(t, ok)
	})

	t.Run("112历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"112_add_payment_order_provider_key_snapshot.sql",
			"d4476c67ceea871aa2d92ee2a603795a742d0379a58cf53938bb9aa559ff9caa",
			"ab871fc02da1eabe0de6ca74a119ee3cea9c727caed30af2ae07a0cd1176d1b8",
		)
		require.True(t, ok)
	})

	t.Run("恢复官方115文件后仍兼容历史与二开数据库checksum", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f",
			"022370762c2dd0ce4f665579b380653478723118886195e9dad481b903195394",
			"72f32dec60e352e652006b0a09ed8720b4c88e4afc177ecde22266a9803d7203",
		} {
			ok := isMigrationChecksumCompatible(
				"115_auth_identity_legacy_external_backfill.sql",
				dbChecksum,
				"022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f",
			)
			require.True(t, ok)
		}
	})

	t.Run("恢复官方116文件后仍兼容历史与二开数据库checksum", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877",
			"aee893b8afb6bbbdb37fc4ecc320438ad6f79b2f5a395e27d45fd6d82a9e0df6",
			"a4db306b0b987459590522ebb08ff9ce42ab1ff5d4f99ec4068c41a51f2236da",
		} {
			ok := isMigrationChecksumCompatible(
				"116_auth_identity_legacy_external_safety_reports.sql",
				dbChecksum,
				"07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488",
			)
			require.True(t, ok)
		}
	})

	t.Run("119历史checksum可兼容占位文件", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e",
		)
		require.True(t, ok)
	})

	t.Run("118多个历史checksum都可兼容当前版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"b4a5b7a28f6a7ac67aad214645761e5a8486c83f0f2a1a874d7f67085f83159b",
			"6395ad255f2be2219ad85813b72db6fa7783c81d747e42e098847ef3594f1674",
		} {
			ok := isMigrationChecksumCompatible(
				"118_wechat_dual_mode_and_auth_source_defaults.sql",
				dbChecksum,
				"ed272e0840730b6b8e7838513c4cc8817e8b5e488e27c88b5421adbece5e89c9",
			)
			require.True(t, ok)
		}
	})

	t.Run("120多个历史checksum都可兼容新的notx修复版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61",
			"79ea6127a22e61b3bad6ea29347a8cc3ff005f8b486ef4a51bd04fdda906f931",
		} {
			ok := isMigrationChecksumCompatible(
				"120_enforce_payment_orders_out_trade_no_unique_notx.sql",
				dbChecksum,
				"34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074",
			)
			require.True(t, ok)
		}
	})

	t.Run("123多个历史checksum都可兼容当前版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"ac0d79ca6feb449674f54f593a5eac5f7cc06751047c664b586c1892e19c60d5",
			"ea17c2767b937f08274e091d212a93acb7e2d62521129179830f073a291fbd97",
		} {
			ok := isMigrationChecksumCompatible(
				"123_fix_legacy_auth_source_grant_on_signup_defaults.sql",
				dbChecksum,
				"7faba5ef65051b7ecb215b7fd2351b0828b7c48153ec688ac089c1588d2cde41",
			)
			require.True(t, ok)
		}
	})

	t.Run("119未知checksum不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		)
		require.False(t, ok)
	})
}
