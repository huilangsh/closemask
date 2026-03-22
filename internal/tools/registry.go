package tools

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry() *ToolRegistry {
	reg := &ToolRegistry{
		tools: make(map[string]Tool),
	}

	// 注册默认工具
	reg.Register(&SearchTool{})
	reg.Register(&WeatherTool{})
	reg.Register(&CalculatorTool{})
	reg.Register(&UserInfoTool{})

	return reg
}

// Register 注册工具
func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	log.Printf("工具已注册: %s", tool.Name())
}

// Get 获取工具
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List 列出所有工具
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	return list
}

// Execute 执行工具
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("工具不存在: %s", name)
	}

	return tool.Execute(ctx, args)
}

// ============ 示例工具实现 ============

// SearchTool 搜索工具（模拟）
type SearchTool struct{}

func (t *SearchTool) Name() string {
	return "search"
}

func (t *SearchTool) Description() string {
	return "搜索网络信息"
}

func (t *SearchTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少 query 参数")
	}

	log.Printf("搜索: %s", query)

	// 模拟搜索结果
	return map[string]interface{}{
		"results": []string{
			fmt.Sprintf("关于 '%s' 的搜索结果1", query),
			fmt.Sprintf("关于 '%s' 的搜索结果2", query),
			fmt.Sprintf("关于 '%s' 的搜索结果3", query),
		},
		"count": 3,
	}, nil
}

// WeatherTool 天气工具（模拟）
type WeatherTool struct{}

func (t *WeatherTool) Name() string {
	return "get_weather"
}

func (t *WeatherTool) Description() string {
	return "查询天气信息"
}

func (t *WeatherTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	city, ok := args["city"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少 city 参数")
	}

	log.Printf("查询天气: %s", city)

	// 模拟天气结果
	return map[string]interface{}{
		"city":        city,
		"temperature": 25,
		"weather":     "晴",
		"humidity":    60,
		"wind":        "东北风 3级",
	}, nil
}

// CalculatorTool 计算器工具
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string {
	return "calculate"
}

func (t *CalculatorTool) Description() string {
	return "执行数学计算"
}

func (t *CalculatorTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	expr, ok := args["expression"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少 expression 参数")
	}

	log.Printf("计算: %s", expr)

	// 简单模拟：返回表达式
	return map[string]interface{}{
		"expression": expr,
		"result":    "计算结果（模拟）",
	}, nil
}

// UserInfoTool 用户信息工具（模拟，返回 PII）
type UserInfoTool struct{}

func (t *UserInfoTool) Name() string {
	return "get_user_info"
}

func (t *UserInfoTool) Description() string {
	return "查询用户信息（包含敏感数据）"
}

func (t *UserInfoTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	userID, ok := args["user_id"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少 user_id 参数")
	}

	log.Printf("查询用户信息: %s", userID)

	// 模拟返回包含 PII 的用户信息
	// 这些 PII 需要被遮罩
	return map[string]interface{}{
		"user_id":    userID,
		"name":       "张三",
		"phone":      "13800138000",
		"email":      "zhangsan@example.com",
		"id_card":    "110101199003077777",
		"address":    "北京市朝阳区XX街道123号",
		"bank_card":  "6225880212345678",
	}, nil
}
