package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionResetCardGrant stores a batch of reset cards granted to one user.
type SubscriptionResetCardGrant struct {
	ent.Schema
}

func (SubscriptionResetCardGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_card_grants"},
	}
}

func (SubscriptionResetCardGrant) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (SubscriptionResetCardGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.Int("issued_count").Positive(),
		field.Int("remaining_count").NonNegative(),
		field.Time("expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("status").
			MaxLen(20).
			Default("active"),
		field.String("source").
			MaxLen(30).
			Default("admin_grant"),
		field.String("request_id").
			MaxLen(64).
			Optional().
			Nillable().
			Immutable().
			Comment("Internal idempotency recovery identifier; never exposed by the API"),
		field.Int64("issued_by").
			Optional().
			Nillable(),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (SubscriptionResetCardGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("subscription_reset_card_grants").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("subscription_reset_card_grants").
			Field("group_id").
			Unique().
			Required(),
		edge.From("issuer", User.Type).
			Ref("issued_subscription_reset_card_grants").
			Field("issued_by").
			Unique(),
		edge.To("usages", SubscriptionResetCardUsage.Type),
	}
}

func (SubscriptionResetCardGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("group_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
		index.Fields("request_id").
			Unique().
			StorageKey("idx_subscription_reset_card_grants_request").
			Annotations(entsql.IndexWhere("request_id IS NOT NULL")),
		index.Fields("user_id", "group_id", "status", "expires_at").
			Annotations(entsql.IndexWhere("remaining_count > 0")),
	}
}
