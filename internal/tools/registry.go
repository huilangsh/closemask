package tools

import (
	"context"
	"fmt"
	"log"
	"sort"
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
	reg.Register(&CheckUserStatusTool{})
	reg.Register(&GetBalanceTool{})
	reg.Register(&TransferMoneyTool{})
	reg.Register(&GetTransactionsTool{})
	reg.Register(&ChangePasswordTool{})

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

// List 列出所有工具（按名称排序）
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	// 按名称排序，确保返回顺序一致
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
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
	phone, _ := args["phone"].(string)
	log.Printf("查询用户信息: phone=%s", phone)

	// 模拟返回包含 PII 的用户信息（这些 PII 需要被遮罩）
	return map[string]interface{}{
		"status": "success",
		"user": map[string]interface{}{
			"phone":    phone,                                     // [TEST DATA]
			"email":    "zhangsan@example.com",                    // [TEST DATA]
			"id_card":  "110101199003077777",                       // [TEST DATA]
			"name":     "张三",
			"balance":  9999.99,
		},
		"api_token":     "sk-1234567890abcdefghijklmnopqrstuvwxyz",   // [TEST DATA]
		"refresh_token": "rt-9876543210zyxwvutsrqponmlkjihgfedcba", // [TEST DATA]
	}, nil
}

// CheckUserStatusTool 检查用户状态工具
type CheckUserStatusTool struct{}

func (t *CheckUserStatusTool) Name() string {
	return "check_user_status"
}

func (t *CheckUserStatusTool) Description() string {
	return "检查用户状态"
}

func (t *CheckUserStatusTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	log.Printf("检查用户状态")

	return map[string]interface{}{
		"status":        "active",
		"last_login":    "2026-03-22T10:30:00Z",
		"session_token": "sess_abc123def456ghi789jkl012mno345pqr678", // [TEST DATA]
	}, nil
}

// GetBalanceTool 查询余额工具
type GetBalanceTool struct{}

func (t *GetBalanceTool) Name() string {
	return "get_balance"
}

func (t *GetBalanceTool) Description() string {
	return "查询账户余额"
}

func (t *GetBalanceTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	phone, _ := args["phone"].(string)
	log.Printf("查询余额: phone=%s", phone)

	return map[string]interface{}{
		"balance":       8888.88,
		"currency":      "CNY",
		"account_token": "acc_token_xyz123abc456", // [TEST DATA]
	}, nil
}

// TransferMoneyTool 转账工具
type TransferMoneyTool struct{}

func (t *TransferMoneyTool) Name() string {
	return "transfer_money"
}

func (t *TransferMoneyTool) Description() string {
	return "执行转账操作"
}

func (t *TransferMoneyTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	fromPhone, _ := args["from_phone"].(string)
	toPhone, _ := args["to_phone"].(string)
	amount, _ := args["amount"].(float64)
	log.Printf("转账: from=%s to=%s amount=%.2f", fromPhone, toPhone, amount)

	return map[string]interface{}{
		"success":            true,
		"transaction_id":     "txn_20260322_0012345678",  // [TEST DATA]
		"confirmation_code":  "CNF-987654",
		"sms_verification":   "sms_code_abc123",           // [TEST DATA]
		"from_phone":         fromPhone,
		"to_phone":           toPhone,
		"amount":             amount,
	}, nil
}

// GetTransactionsTool 查询交易记录工具
type GetTransactionsTool struct{}

func (t *GetTransactionsTool) Name() string {
	return "get_transactions"
}

func (t *GetTransactionsTool) Description() string {
	return "查询交易记录"
}

func (t *GetTransactionsTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	phone, _ := args["phone"].(string)
	log.Printf("查询交易记录: phone=%s", phone)

	return []interface{}{
		map[string]interface{}{
			"date":   "2026-03-22",
			"type":   "转账",
			"amount": -1000.00,
			"to":     "王五 (13600136000)", // [TEST DATA]
			"txn_id": "txn_20260322_0012345678",
		},
		map[string]interface{}{
			"date":   "2026-03-21",
			"type":   "收入",
			"amount": 5000.00,
			"from":   "工资收入",
			"txn_id": "txn_20260321_99887766",
		},
	}, nil
}

// ChangePasswordTool 修改密码工具
type ChangePasswordTool struct{}

func (t *ChangePasswordTool) Name() string {
	return "change_password"
}

func (t *ChangePasswordTool) Description() string {
	return "修改用户密码"
}

func (t *ChangePasswordTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	phone, _ := args["phone"].(string)
	code, _ := args["verification_code"].(string)
	log.Printf("修改密码: phone=%s code=%s", phone, code)

	return map[string]interface{}{
		"success": true,
		"message": "密码修改成功，请使用新密码登录",
	}, nil
}
