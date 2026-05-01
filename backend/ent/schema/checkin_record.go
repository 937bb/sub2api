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

// CheckinRecord 每日签到记录。
//
// 防刷机制：user_id + checkin_date 唯一索引。
type CheckinRecord struct {
	ent.Schema
}

func (CheckinRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "checkin_records"},
	}
}

func (CheckinRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("用户ID"),
		field.Time("checkin_date").
			SchemaType(map[string]string{dialect.Postgres: "date"}).
			Comment("签到日期"),
		field.Int("streak").
			Default(1).
			Comment("连续签到天数（含当天）"),
		field.Float("base_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("基础签到奖励"),
		field.Float("milestone_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("里程碑奖励"),
		field.Float("total_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("总奖励"),
		field.String("balance_type").
			MaxLen(20).
			Default("permanent").
			Comment("奖励余额类型：permanent / expirable"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("创建时间"),
	}
}

func (CheckinRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("checkin_records").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (CheckinRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "checkin_date").
			Unique(),
		index.Fields("user_id"),
	}
}
