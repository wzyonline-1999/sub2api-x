package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionResetPreference stores a user's automatic reset choice per group.
type SubscriptionResetPreference struct {
	ent.Schema
}

func (SubscriptionResetPreference) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_preferences"},
	}
}

func (SubscriptionResetPreference) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (SubscriptionResetPreference) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.Bool("auto_use_enabled").Default(false),
	}
}

func (SubscriptionResetPreference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("subscription_reset_preferences").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("subscription_reset_preferences").
			Field("group_id").
			Unique().
			Required(),
	}
}

func (SubscriptionResetPreference) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "group_id").Unique(),
	}
}
