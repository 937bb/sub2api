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

// BalanceEntry 余额明细：每笔余额变动的详细记录。
// 区分永久余额（permanent）和有效期余额（expirable）。
//
// 删除策略：硬删除
// 余额明细是账务数据，一般不删除；过期条目通过 expired 标记位处理。
type BalanceEntry struct {
	ent.Schema
}

func (BalanceEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "balance_entries"},
	}
}

func (BalanceEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("用户ID"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("变动金额（正=入账，负=扣减记录）"),
		field.Float("remaining").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0).
			Comment("该笔入账余额剩余可用金额"),
		field.String("balance_type").
			MaxLen(20).
			Default("permanent").
			Comment("余额类型：permanent=永久，expirable=有有效期"),
		field.String("source").
			MaxLen(40).
			Comment("来源类型"),
		field.String("note").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("").
			Comment("描述/备注"),
		field.Time("expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("过期时间，仅 expirable 类型使用"),
		field.Bool("expired").
			Default(false).
			Comment("是否已被过期清理任务标记过期"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("创建时间"),
	}
}

func (BalanceEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("balance_entries").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (BalanceEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("expires_at"),
	}
}
