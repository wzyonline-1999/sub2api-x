package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionResetCardUsage is an immutable audit record for one consumed card.
type SubscriptionResetCardUsage struct {
	ent.Schema
}

func (SubscriptionResetCardUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_card_usages"},
	}
}

func (SubscriptionResetCardUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("grant_id"),
		field.Int64("subscription_id"),
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.String("mode").MaxLen(10),
		field.String("request_id").
			MaxLen(128).
			Optional().
			Nillable(),
		field.Float("previous_daily_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("previous_weekly_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("previous_monthly_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Time("previous_daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("previous_weekly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("previous_monthly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("used_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionResetCardUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("grant", SubscriptionResetCardGrant.Type).
			Ref("usages").
			Field("grant_id").
			Unique().
			Required(),
		edge.From("subscription", UserSubscription.Type).
			Ref("subscription_reset_card_usages").
			Field("subscription_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("subscription_reset_card_usages").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("subscription_reset_card_usages").
			Field("group_id").
			Unique().
			Required(),
	}
}

func (SubscriptionResetCardUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("grant_id"),
		index.Fields("subscription_id"),
		index.Fields("user_id", "used_at"),
		index.Fields("group_id", "used_at"),
		index.Fields("request_id").
			Unique().
			Annotations(entsql.IndexWhere("request_id IS NOT NULL")),
	}
}
