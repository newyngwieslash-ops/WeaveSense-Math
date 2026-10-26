package capability

import "yingce/backend/internal/canvas/contract"

const (
	maxAgentNodeTitleRunes   = 240
	maxAgentNodeContentRunes = 16000
	// 坐标绝对值上限：与 app 侧整理工具收敛几何值的范围一致。
	maxAgentNodeCoordLimit = 1e6
)

func BuiltinRegistry() *Registry {
	registry, err := NewRegistry([]Descriptor{
		{
			Type: "text", Version: "1", Label: "文本", DefaultWidth: 340, DefaultHeight: 240,
			Purpose:     "承载普通说明、创意草稿和单段提示词。",
			GoodFor:     []string{"单个创意", "一次性提示词", "临时备注", "快速试验"},
			NotIdealFor: []string{"多镜头脚本", "需要逐镜修改的内容", "需要镜头级资产关系的内容"},
			Tradeoffs:   []string{"创建和编辑最轻量", "没有镜头级字段和逐镜维护能力"},
			InputKind:   "text", Connection: ConnectionPolicy{CanSource: true}, CanUpdate: true,
			SummaryFields: []string{"content"}, DetailFields: []string{"content"},
			PatchFields: editableNodeFields("metadata.content", "正文", "节点正文"),
			CreateMetadata: func(content string) map[string]any {
				return map[string]any{"content": content, "status": "idle", "fontSize": float64(14)}
			},
		},
		{
			Type: "markdown", Version: "1", Label: "Markdown", DefaultWidth: 420, DefaultHeight: 320,
			Purpose:     "承载面向人阅读的方案、脚本草稿和格式化文档。",
			GoodFor:     []string{"创意方案", "脚本草稿", "交付文档", "需要排版的长文本"},
			NotIdealFor: []string{"需要逐镜生成或审核的多镜头内容", "需要绑定媒体资产的结构化流程"},
			Tradeoffs:   []string{"适合阅读和导出", "结构化程度低，不能替代可维护的分镜表"},
			InputKind:   "text", Connection: ConnectionPolicy{CanSource: true}, CanUpdate: true,
			SummaryFields: []string{"content"}, DetailFields: []string{"content"},
			PatchFields: editableNodeFields("metadata.content", "Markdown 正文", "Markdown 正文"),
		},
		{
			Type: "file", Version: "1", Label: "文本文件", DefaultWidth: 420, DefaultHeight: 320,
			Purpose:     "承载上传到画布的 TXT、Markdown 或其它文件资源；正文通过 canvas_read_text 在当前用户权限内读取。",
			GoodFor:     []string{"读取上传小说或剧本原文", "保留原始文件作为改编来源"},
			NotIdealFor: []string{"图片、视频或音频参考", "需要逐镜维护的结构化分镜"},
			Tradeoffs:   []string{"文件正文按需读取并分页返回", "二进制文件不会被当作文本解析"},
			InputKind:   "text", Connection: ConnectionPolicy{CanSource: true},
			SummaryFields: []string{"mimeType"}, DetailFields: []string{"mimeType"},
		},
		generatedMediaDescriptor("image", "2", "图片", 720, 405, "image", ConnectionPolicy{
			CanSource: true, CanTarget: true, CanReference: true, AcceptedInputKinds: []string{"text", "image", "character"},
		}),
		generatedMediaDescriptor("video", "2", "视频", 720, 405, "video", ConnectionPolicy{
			CanSource: true, CanTarget: true, CanReference: true, AcceptedInputKinds: []string{"text", "image", "video", "audio", "character"},
		}),
		generatedMediaDescriptor("audio", "2", "音频", 340, 120, "audio", ConnectionPolicy{
			CanSource: true, CanTarget: true, CanReference: true, MaxInputCount: 1, AcceptedInputKinds: []string{"text", "character"},
		}),
		characterDescriptor(),
		{
			Type: "frame", Version: "1", Label: "背板", DefaultWidth: 760, DefaultHeight: 520,
			Purpose:       "在画布上建立可移动、可折叠的视觉分区，用来归组相关节点；背板本身不承载创作正文或生成结果。",
			GoodFor:       []string{"按场景或镜头组归拢脚本、参考图和生成结果", "按前期策划、制作、交付等阶段划分工作区", "为大型画布建立清晰分区，便于移动或折叠整组内容"},
			NotIdealFor:   []string{"承载结构化镜头数据（应使用分镜脚本节点）", "存放需要 Agent 单独读写的正文（应使用文本或 Markdown 节点）", "替代具体业务节点或媒体生成节点"},
			Tradeoffs:     []string{"改善空间组织但不增加内容结构或生成能力"},
			SummaryFields: []string{"label"}, DetailFields: []string{"label"},
			CreateMetadata: func(string) map[string]any {
				return map[string]any{"frame": map[string]any{"collapsed": false, "expandedWidth": float64(760), "expandedHeight": float64(520)}}
			},
		},
		{
			Type: "batch-table", Version: "1", Label: "批量创作表", DefaultWidth: 1280, DefaultHeight: 560,
			Purpose:     "面向电商批量换装和创意生图的结构化任务表；每行绑定最多六组画布图片、可用 @参考图1 等位置引用编写独立提示词，也可设置全局提示词覆盖各行，并追踪生成结果。",
			GoodFor:     []string{"商品与模特批量换装", "同一商品多场景创意图", "多组参考图组合生成", "批量结果追踪与失败重试"},
			NotIdealFor: []string{"通用数据库或库存管理", "单张图片快速试验", "多镜头叙事连续性"},
			Tradeoffs:   []string{"参考图必须先作为图片节点进入画布", "批量提交会产生多项生成任务，执行前必须确认模型、数量和费用"},
			Actions:     []string{"read_rows", "append_row", "update_row", "remove_row", "set_operation", "set_concurrency", "add_reference_column", "remove_reference_column", "set_global_prompt", "preview_batch_generation"},
			InputKind:   "text",
			Connection:  ConnectionPolicy{CanSource: true, CanTarget: true, AcceptedInputKinds: []string{"image"}},
			CanUpdate:   true, SummaryFields: []string{"batchTable"}, DetailFields: []string{"batchTable"}, ProjectionKind: "batch_table", ProjectionField: "batchTable",
			PatchFields: func() map[string]PatchField {
				fields := map[string]PatchField{
					"title": {Path: "title", Kind: patchKindString, Label: "节点名称", Order: 10, Description: "批量创作表标题", MaxRunes: maxAgentNodeTitleRunes},
				}
				for key, field := range positionPatchFields() {
					fields[key] = field
				}
				return fields
			}(),
			CreateMetadata: func(string) map[string]any {
				return map[string]any{
					"status": "idle",
					"batchTable": map[string]any{
						"operation": "try_on", "concurrency": float64(10),
						"referenceColumns": []any{
							map[string]any{"id": "reference-1", "label": "参考图 1"},
							map[string]any{"id": "reference-2", "label": "参考图 2"},
							map[string]any{"id": "reference-3", "label": "参考图 3"},
						},
						"rows": []any{},
					},
				}
			},
		},
		{
			Type: "script", Version: "1", Label: "分镜脚本", DefaultWidth: 920, DefaultHeight: 360,
			Purpose:       "维护结构化的多镜头脚本，支持镜头级审查、修改和媒体关联。",
			GoodFor:       []string{"多镜头规划", "镜头连续性", "逐镜审查和微调", "逐镜生成图片或视频", "需要他人接手维护的内容"},
			NotIdealFor:   []string{"只有一个画面的快速试验", "一次性临时提示词", "仅需要阅读排版的普通文档"},
			Tradeoffs:     []string{"前期录入成本高于文本节点", "但能保留镜头级结构、资产关系和后续维护能力"},
			Actions:       []string{"read_rows", "append_row", "update_row", "remove_row", "generate_storyboard", "connect_assets", "update_node"},
			InputKind:     "text",
			Connection:    ConnectionPolicy{CanSource: true, CanTarget: true, AcceptedInputKinds: []string{"audio", "character", "image", "text", "video"}},
			CanUpdate:     true,
			SummaryFields: []string{"storyboard"}, DetailFields: []string{"storyboard"}, ProjectionKind: "storyboard", ProjectionField: "storyboard",
			PatchFields: titleAndPositionPatchFields(),
			CreateMetadata: func(string) map[string]any {
				return map[string]any{"status": "idle", "workflowKind": "script", "storyboard": map[string]any{"rows": []any{}, "visibleColumns": []any{"shotNumber", "durationSeconds", "videoMotionPrompt", "dialogue", "assets"}, "referenceNodeIds": []any{}}}
			},
		},
		{
			Type: "data-source", Version: "1", Label: "数据源", DefaultWidth: 420, DefaultHeight: 340,
			Purpose:       "承载结构化数据源（JSON、CSV、TXT、Markdown、情报条目），供下游逻辑与集合节点运算或驱动媒体生成。",
			GoodFor:       []string{"结构化 JSON/CSV 数据输入", "情报条目集合输入", "驱动逻辑分支与集合运算"},
			NotIdealFor:   []string{"直接生成图片或视频", "长篇小说排版展示"},
			Tradeoffs:     []string{"支持按条目解析与字段提取", "不直接发起模型生成任务"},
			InputKind:     "text",
			Connection:    ConnectionPolicy{CanSource: true, CanTarget: true, AcceptedInputKinds: []string{"text"}},
			CanUpdate:     true,
			SummaryFields: []string{"content", "dataSource"}, DetailFields: []string{"content", "dataSource"},
			PatchFields: func() map[string]PatchField {
				fields := editableNodeFields("metadata.content", "数据源内容", "JSON / CSV / Markdown / TXT 原始文本")
				fields["format"] = PatchField{Path: "metadata.dataSource.format", Kind: patchKindString, Label: "数据格式", Order: 25, Description: "json | csv | txt | markdown | intel", MaxRunes: 32}
				fields["sourceBindingKind"] = PatchField{Path: "metadata.dataSource.sourceBinding.kind", Kind: patchKindString, Label: "绑定类型", Order: 26, Description: "static | intel_query | topic | monitor | mcp_tool", MaxRunes: 32}
				fields["sourceBindingRefId"] = PatchField{Path: "metadata.dataSource.sourceBinding.refId", Kind: patchKindString, Label: "绑定ID", Order: 27, Description: "专题/监测/文档ID", MaxRunes: 120}
				setFieldSchema(fields, "sourceBindingKind", enumSchema("static", "intel_query", "topic", "monitor", "mcp_tool"))
				fields["sourceBindingQuery"] = structuredField("metadata.dataSource.sourceBinding.query", "绑定查询", 28, map[string]any{"type": "string"})
				fields["sourceBindingSourceIds"] = structuredField("metadata.dataSource.sourceBinding.sourceIds", "来源ID", 29, stringArraySchema())
				fields["sourceBindingTimeWindow"] = structuredField("metadata.dataSource.sourceBinding.timeWindow", "时间窗口", 32, objectSchema(map[string]any{
					"kind": enumSchema("7d", "30d", "custom"), "from": nonemptyStringSchema(), "to": nonemptyStringSchema(), "timezone": nonemptyStringSchema(),
				}, "kind"))
				fields["sourceBindingToolName"] = structuredField("metadata.dataSource.sourceBinding.toolName", "工具名", 33, nonemptyStringSchema())
				// MCP tool arguments have tool-owned keys; the binding envelope remains closed.
				fields["sourceBindingToolArgs"] = structuredField("metadata.dataSource.sourceBinding.toolArgs", "工具参数", 34, map[string]any{"type": "object", "additionalProperties": true})
				fields["sourceBindingToolVersion"] = structuredField("metadata.dataSource.sourceBinding.toolVersion", "工具版本", 35, map[string]any{"type": "integer", "minimum": 1})
				fields["sourceBindingSchemaHash"] = structuredField("metadata.dataSource.sourceBinding.schemaHash", "工具模式哈希", 36, nonemptyStringSchema())
				fields["sourceBindingOutputSchema"] = structuredField("metadata.dataSource.sourceBinding.outputSchema", "输出模式", 37, map[string]any{"type": "object", "additionalProperties": true})
				fields["sourceBindingWindowArguments"] = structuredField("metadata.dataSource.sourceBinding.windowArguments", "窗口参数映射", 38, objectSchema(map[string]any{"from": nonemptyStringSchema(), "to": nonemptyStringSchema()}, "from", "to"))
				return fields
			}(),
			CreateMetadata: func(content string) map[string]any {
				return map[string]any{"content": content, "status": "idle", "dataSource": map[string]any{"format": "json", "rawText": content}}
			},
		},
		{
			Type: "logic", Version: "1", Label: "逻辑与建模算子", DefaultWidth: 400, DefaultHeight: 320,
			Purpose:       "对上游数据执行 If/Else 分支、集合运算（交/并/差）、过滤、提取，以及指标计算（compute）、分组聚合（aggregate）、多源关联（join）与 Top-N 排序（sort_limit）。",
			GoodFor:       []string{"If/Else 分流与条件过滤", "多路数据交集、并集、差集与关联拼接（join）", "污水折算千人消费量等指标计算（compute）、区域分组聚合（aggregate）与高危 Top-N（sort_limit）"},
			NotIdealFor:   []string{"承载静态长文档", "直接替代模型生成节点"},
			Tradeoffs:     []string{"前后端同构确定性求值，不执行任意非受控代码", "当连接至下游生成节点的逻辑分支为空（0 条命中）时，会自动门控拦截下游生成"},
			InputKind:     "text",
			Connection:    ConnectionPolicy{CanSource: true, CanTarget: true, AcceptedInputKinds: []string{"audio", "character", "image", "text", "video"}},
			CanUpdate:     true,
			SummaryFields: []string{"content", "logic"}, DetailFields: []string{"content", "logic"},
			PatchFields: func() map[string]PatchField {
				fields := titleAndPositionPatchFields()
				fields["operator"] = PatchField{Path: "metadata.logic.operator", Kind: patchKindString, Label: "运算类型", Order: 20, Description: "if_else | intersect | union | difference | filter | json_extract | compute | aggregate | join | sort_limit", MaxRunes: 32}
				fields["fieldPath"] = PatchField{Path: "metadata.logic.fieldPath", Kind: patchKindString, Label: "字段路径", Order: 21, Description: "点路径字段名，如 score 或 meta.tag", MaxRunes: 120}
				fields["comparator"] = PatchField{Path: "metadata.logic.comparator", Kind: patchKindString, Label: "比较符", Order: 22, Description: "contains | not_contains | equals | not_equals | gt | lt | not_empty | is_empty | regex", MaxRunes: 32}
				fields["compareValue"] = PatchField{Path: "metadata.logic.compareValue", Kind: patchKindString, Label: "比较值", Order: 23, Description: "条件比较目标值", MaxRunes: 500}
				fields["matchKey"] = PatchField{Path: "metadata.logic.matchKey", Kind: patchKindString, Label: "匹配键", Order: 24, Description: "集合运算匹配字段，如 id；Join 使用 joinKeys", MaxRunes: 120}
				fields["expression"] = PatchField{Path: "metadata.logic.expression", Kind: patchKindString, Label: "计算公式", Order: 25, Description: "如 wbe_consumption_mg_1000p 或四则表达式", MaxRunes: 240}
				fields["outputField"] = PatchField{Path: "metadata.logic.outputField", Kind: patchKindString, Label: "输出字段", Order: 26, Description: "计算或聚合输出字段名", MaxRunes: 80}
				fields["groupBy"] = structuredField("metadata.logic.groupBy", "分组字段", 27, stringArraySchema())
				fields["metrics"] = structuredField("metadata.logic.metrics", "聚合指标", 28, arraySchema(objectSchema(map[string]any{
					"field": map[string]any{"type": "string"}, "fn": enumSchema("count", "sum", "avg", "max", "min", "p95"), "as": nonemptyStringSchema(),
				}, "field", "fn", "as")))
				fields["joinMode"] = PatchField{Path: "metadata.logic.joinMode", Kind: patchKindString, Label: "关联模式", Order: 30, Description: "inner | left", MaxRunes: 16}
				fields["joinKeys"] = structuredField("metadata.logic.joinKeys", "关联键", 31, arraySchema(objectSchema(map[string]any{"left": nonemptyStringSchema(), "right": nonemptyStringSchema()}, "left", "right")))
				fields["dedupeKeys"] = structuredField("metadata.logic.dedupeKeys", "去重字段", 34, stringArraySchema())
				fields["limit"] = structuredField("metadata.logic.limit", "数量上限", 35, map[string]any{"type": "integer", "minimum": 1})
				fields["periodComparison"] = structuredField("metadata.logic.periodComparison", "周期比较", 36, objectSchema(map[string]any{
					"regionField": nonemptyStringSchema(), "substanceField": nonemptyStringSchema(), "periodField": nonemptyStringSchema(), "baselinePeriodField": nonemptyStringSchema(), "valueField": nonemptyStringSchema(),
				}, "regionField", "substanceField", "periodField", "baselinePeriodField", "valueField"))
				fields["sortField"] = PatchField{Path: "metadata.logic.sortField", Kind: patchKindString, Label: "排序字段", Order: 32, Description: "Top-N 排序字段", MaxRunes: 80}
				fields["sortOrder"] = PatchField{Path: "metadata.logic.sortOrder", Kind: patchKindString, Label: "排序方向", Order: 33, Description: "desc | asc", MaxRunes: 8}
				setFieldSchema(fields, "operator", enumSchema("if_else", "intersect", "union", "difference", "filter", "json_extract", "compute", "aggregate", "join", "sort_limit"))
				setFieldSchema(fields, "joinMode", enumSchema("left", "inner"))
				setFieldSchema(fields, "sortOrder", enumSchema("desc", "asc"))
				return fields
			}(),
			CreateMetadata: func(content string) map[string]any {
				return map[string]any{"content": content, "status": "idle", "logic": map[string]any{"operator": "if_else", "fieldPath": "score", "comparator": "gt", "compareValue": "80", "matchKey": "id"}}
			},
		},
		{
			Type: "chart", Version: "1", Label: "图表", DefaultWidth: 420, DefaultHeight: 280,
			Purpose:       "将上游数据源、逻辑聚合或文本节点的 JSON 数组/CSV 渲染为柱状图或折线图，用于区域趋势、排行与量化研判可视化。",
			GoodFor:       []string{"区域指标排行柱状图", "时间序列趋势折线图", "连接 aggregate / compute / sort_limit 算子直接可视化"},
			NotIdealFor:   []string{"非结构化散文文本", "驱动下游媒体生成"},
			Tradeoffs:     []string{"作为可视化终端节点，接收结构化数组或 CSV"},
			InputKind:     "text",
			Connection:    ConnectionPolicy{CanSource: false, CanTarget: true, AcceptedInputKinds: []string{"text"}},
			CanUpdate:     true,
			SummaryFields: []string{"content", "chartKind", "xKey", "series"}, DetailFields: []string{"content", "chartKind", "xKey", "series"},
			PatchFields: func() map[string]PatchField {
				fields := editableNodeFields("metadata.content", "图表数据", "JSON 数组或 CSV 文本（留空则直接读取上游节点输出）")
				fields["chartKind"] = PatchField{Path: "metadata.chartKind", Kind: patchKindString, Label: "图表类型", Order: 25, Description: "bar | line", MaxRunes: 16}
				setFieldSchema(fields, "chartKind", enumSchema("bar", "line"))
				fields["xKey"] = structuredField("metadata.xKey", "横轴字段", 26, nonemptyStringSchema())
				fields["series"] = structuredField("metadata.series", "数值系列", 27, stringArraySchema())
				return fields
			}(),
			CreateMetadata: func(content string) map[string]any {
				return map[string]any{"content": content, "status": "idle", "chartKind": "bar"}
			},
		},
		{
			Type: "skill", Version: "1", Label: "Skill 技能卡", DefaultWidth: 360, DefaultHeight: 280,
			Purpose:       "承载技能模板与结构化提示词规约，可接收上游数据源或逻辑分支数据，组合输出给下游生成节点。",
			GoodFor:       []string{"在数据流中挂接特定技能提示词模板", "串联数据源/逻辑节点与下游生成节点"},
			NotIdealFor:   []string{"直接替代图片或视频生成节点"},
			Tradeoffs:     []string{"作为技能指令与上游数据的汇聚转换节点，本身不产生计费媒体产物"},
			InputKind:     "text",
			Connection:    ConnectionPolicy{CanSource: true, CanTarget: true, AcceptedInputKinds: []string{"text"}},
			CanUpdate:     true,
			SummaryFields: []string{"content", "skillId", "skillVersion", "skillHash", "skillName", "skillDescription"}, DetailFields: []string{"content", "skillId", "skillVersion", "skillHash", "skillName", "skillDescription", "skillPrompts"},
			PatchFields: func() map[string]PatchField {
				fields := editableNodeFields("metadata.content", "技能指令", "技能核心指令或提示词模板")
				fields["skillId"] = PatchField{Path: "metadata.skillId", Kind: patchKindString, Label: "技能ID", Order: 25, Description: "关联技能标识", MaxRunes: 120}
				fields["skillName"] = PatchField{Path: "metadata.skillName", Kind: patchKindString, Label: "技能名称", Order: 26, Description: "技能显示名称", MaxRunes: 120}
				fields["skillDescription"] = PatchField{Path: "metadata.skillDescription", Kind: patchKindString, Label: "技能说明", Order: 27, Description: "技能用途简述", MaxRunes: 500}
				fields["skillVersion"] = structuredField("metadata.skillVersion", "技能版本", 28, nonemptyStringSchema())
				fields["skillHash"] = structuredField("metadata.skillHash", "技能哈希", 29, nonemptyStringSchema())
				return fields
			}(),
			CreateMetadata: func(content string) map[string]any {
				prompts := []any{}
				if content != "" {
					prompts = []any{content}
				}
				return map[string]any{"content": content, "status": "idle", "skillName": "Skill 节点", "skillDescription": "", "skillPrompts": prompts}
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return registry
}

// characterDescriptor 是角色卡能力：画布上是 text + metadata.workflowKind=character 的节点，
// 设定、三视图与声音都来自账号内的角色资产而非节点正文。它可以被发现、精读、连线和引用，
// 但不能用 add_node 凭空创建：必须经 canvas_create_character 用真实图片/音频建角色资产，名称和设定只随角色资产更新。
func characterDescriptor() Descriptor {
	return Descriptor{
		Type: "character", Version: "1", Label: "角色卡", DefaultWidth: 264, DefaultHeight: 352,
		Purpose:     "引用账号角色库中的角色资产，统一提供角色设定、三视图/形象图和绑定声音，用来保持人物在多次生成中的一致性。",
		GoodFor:     []string{"图片或视频生成时锁定人物外观", "多镜头保持同一角色一致", "角色配音时使用绑定声音", "只取角色文字设定作为提示词来源"},
		NotIdealFor: []string{"临时一次性的人物描述（直接写进提示词或文本节点）", "用 add_node 新建（应使用 canvas_create_character 打包形象图片与声音）", "承载普通正文（节点正文不是角色设定）"},
		Tradeoffs:   []string{"设定与媒体随角色资产版本变化，提交时会校验版本", "未绑定形象或声音时对应引用不可用，需先读取 character.imageReference/audioReference"},
		Actions:     []string{"read_character", "use_as_reference", "use_as_text_source"},
		InputKind:   "character",
		Connection:  ConnectionPolicy{CanSource: true, CanReference: true},
		CanUpdate:   true,
		PatchFields: positionPatchFields(),
		Variant:     &NodeVariant{BaseType: "text", WorkflowKind: "character"},
	}
}

// generatedMediaDescriptor 根据节点类型、显示尺寸、生成模式和连接规则创建媒体能力描述。
// 返回读写字段及创建默认值；首尾帧只对视频开放，避免其他节点接受无效视频参数。
func generatedMediaDescriptor(nodeType, version, label string, width, height float64, generationMode string, connection ConnectionPolicy) Descriptor {
	semantics := generatedMediaSemantics(nodeType)
	fields := editableNodeFields("metadata.generationSpec.prompt", "提示词", "生成合同中的当前提示词；这是 Agent 唯一读写的媒体提示词")
	readFields := []string{"prompt", "assetTags", "referenceNodeIds"}
	if generationMode == "video" {
		for i, frame := range []struct{ key, label string }{
			{"videoStartFrameNodeId", "首帧"}, {"videoEndFrameNodeId", "尾帧"},
		} {
			fields[frame.key] = PatchField{Path: "metadata." + frame.key, Kind: patchKindString, Label: frame.label, Order: 40 + i, MaxRunes: 80, Description: "已连接图片ID；空字符串取消"}
			readFields = append(readFields, frame.key)
		}
	}
	return Descriptor{
		Type: nodeType, Version: version, Label: label, DefaultWidth: width, DefaultHeight: height,
		Purpose: semantics.Purpose, GoodFor: semantics.GoodFor, NotIdealFor: semantics.NotIdealFor,
		Tradeoffs: semantics.Tradeoffs, Actions: semantics.Actions,
		InputKind: nodeType, GenerationMode: generationMode, Connection: connection, CanUpdate: true,
		SummaryFields:  readFields,
		DetailFields:   readFields,
		PatchFields:    fields,
		CreateMetadata: func(prompt string) map[string]any { return generatedMetadata(generationMode, prompt) },
	}
}

type generatedMediaCapabilitySemantics struct {
	Purpose     string
	GoodFor     []string
	NotIdealFor []string
	Tradeoffs   []string
	Actions     []string
}

func generatedMediaSemantics(nodeType string) generatedMediaCapabilitySemantics {
	switch nodeType {
	case "image":
		return generatedMediaCapabilitySemantics{
			Purpose:     "生成或承载一个静态画面，并作为后续图片或视频生成的真实参考素材。",
			GoodFor:     []string{"单张图片生成", "有参考图的图片生成", "分镜首帧和关键帧", "需要复用的视觉素材"},
			NotIdealFor: []string{"承载多镜头脚本结构", "表达镜头运动或时间变化", "代替分镜表维护镜头连续性"},
			Tradeoffs:   []string{"一个节点对应一个静态媒体目标", "参考图数量和生成方式必须再由模型目录能力匹配"},
			Actions:     []string{"generate_media", "update_prompt", "use_as_reference"},
		}
	case "video":
		return generatedMediaCapabilitySemantics{
			Purpose:     "生成或承载一个连续视频片段；可按实际参考素材执行文生视频、图生视频或多图生视频。",
			GoodFor:     []string{"单镜头文生视频", "单图生视频", "多图参考视频", "已有视频或音频参与的视频生成"},
			NotIdealFor: []string{"承载整部多镜头脚本", "用一个节点代替逐镜审查和维护", "未查询模型能力就假定支持任意参考数量"},
			Tradeoffs:   []string{"一个节点通常对应一个可独立生成和审核的视频片段", "参考类型、数量、时长、画幅、音频和价格受当前模型目录约束"},
			Actions:     []string{"generate_media", "update_prompt", "use_as_reference"},
		}
	case "audio":
		return generatedMediaCapabilitySemantics{
			Purpose:     "生成或承载一个音频素材，用于配音、音乐、音效或视频参考输入。",
			GoodFor:     []string{"文本转语音", "配音", "音乐或音效素材", "为视频提供音频参考"},
			NotIdealFor: []string{"承载分镜结构", "表达画面构图或镜头运动", "代替视频节点"},
			Tradeoffs:   []string{"一个节点对应一个音频目标且最多接收一个文本输入", "音频类型、时长和价格仍以模型目录及任务准入为准"},
			Actions:     []string{"generate_media", "update_prompt", "use_as_reference"},
		}
	default:
		return generatedMediaCapabilitySemantics{}
	}
}

func titleAndPositionPatchFields() map[string]PatchField {
	fields := map[string]PatchField{
		"title": {
			Path: "title", Kind: patchKindString, Label: "节点名称", Order: 10, Description: "节点标题", MaxRunes: maxAgentNodeTitleRunes,
		},
	}
	for key, field := range positionPatchFields() {
		fields[key] = field
	}
	return fields
}

func editableNodeFields(contentPath, contentLabel, contentDescription string) map[string]PatchField {
	fields := titleAndPositionPatchFields()
	fields["content"] = PatchField{
		Path: contentPath, Kind: patchKindString, Label: contentLabel, Order: 20, Description: contentDescription, MaxRunes: maxAgentNodeContentRunes,
	}
	return fields
}

// positionPatchFields 是坐标字段：模型可以直接指定节点位置（微调），批量整理走 canvas_arrange_nodes。
// 坐标上限与整理侧的收敛范围一致，避免写进离谱的几何值。
func positionPatchFields() map[string]PatchField {
	return map[string]PatchField{
		"x": {Path: "position.x", Kind: patchKindNumber, Label: "横坐标", Order: 30, Limit: maxAgentNodeCoordLimit, Description: "画布横坐标（像素）；与 y 一起移动节点，通常用 canvas_arrange_nodes 批量整理"},
		"y": {Path: "position.y", Kind: patchKindNumber, Label: "纵坐标", Order: 31, Limit: maxAgentNodeCoordLimit, Description: "画布纵坐标（像素）；与 x 一起移动节点，通常用 canvas_arrange_nodes 批量整理"},
	}
}

func generatedMetadata(mode, prompt string) map[string]any {
	spec := contract.GenerationSpec{
		Version:           contract.GenerationVersion,
		Mode:              mode,
		Prompt:            prompt,
		Options:           contract.Options{},
		ReferenceBindings: []contract.ReferenceBinding{},
		TextInputMode:     "append-sources",
	}
	metadata, err := spec.NodeMetadata()
	if err != nil {
		panic(err)
	}
	metadata["content"] = ""
	metadata["status"] = "idle"
	return metadata
}
