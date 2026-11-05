# 模拟充值与复式账本验证

当前流程：模拟转账 → `detected` → `confirmed` → `credited`。
确认的高度和哈希仍使用模拟值，不会访问真实 Ethereum RPC。

## 表结构与迁移

- `013`、`014` 是已执行过的单向流水和余额表迁移，继续保留。
- `015_create_double_entry_ledger.sql` 检查旧数据一致性，将旧表归档为 `ledger_legacy`、`balances_legacy`，转换成复式账本。整个文件必须在一个事务内执行。
- `016_drop_legacy_accounting_tables.sql` 删除已经完成转换的两张旧表，当前余额和复式账本保留。历史迁移文件继续保留，方便从空库重建。
- `accounts`：平台托管资产账户，以及每个用户的可用、冻结负债账户；按资产分别建立。
- `ledger_transactions`：业务主记录，`business_type + business_id` 唯一；充值还通过唯一 `deposit_id` 关联原记录。
- `ledger`：分录，一笔充值包含平台托管借方、用户可用贷方，各记相同金额。
- `balances`：按账户保存余额。资产账户借方增加、贷方减少；用户负债账户贷方增加、借方减少。

金额始终使用资产最小整数单位。USDT 显示金额需要除以 1,000,000。
冻结、解冻、提现和冲正的业务类型已预留，但没有相应的业务入口。本轮只实现充值；冻结账户不是已实现提现冻结功能的声明。

## 业务代码

- `internal/asset/credit.go`：锁定 confirmed 充值，确保账户存在，按账户 ID 顺序加锁，创建业务主记录、两条分录、更新两个账户余额，最后标记 credited。
- `internal/asset/balance.go`：`GetUserBalance` 返回 `Available`、`Frozen`，两者之和是用户总余额。
- 已 credited 的充值再次处理返回 false，不增加余额。
- 数据库在提交前检查借贷平衡、同种资产、充值用户与账户归属，以及余额与分录累计一致；金额不能为负或包含小数。
- 已记账的主记录、分录及账户身份不可修改或删除。已 credited 的充值不能更改其金额、用户、资产、来源或状态；后续纠错应新增反向分录。

## 当前测试数据的预期

10 个用户、一种资产：21 个账户和余额记录（1 个平台、10 个可用、10 个冻结）。
现有 60 笔充值对应 60 笔入账主记录、120 条分录，充值状态仍是 credited。
用户可用余额与迁移前一致；全部冻结余额为 0。
平台托管余额与用户可用余额合计均为原始整数 `766779175758`，即 `766779.175758 USDT`。
借方合计与贷方合计也分别为这个数，不能将所有账户余额相加当成用户总资产。
这只是模拟账本，不证明平台实际钱包持有这些链上资产。

## 本地运行与核对

已有数据库只升级一次，勿重复执行迁移：

```sh
docker compose exec -T postgres psql -U rick -d reliable_chain -v ON_ERROR_STOP=1 --single-transaction -f /dev/stdin < migrations/015_create_double_entry_ledger.sql
docker compose exec -T postgres psql -U rick -d reliable_chain -v ON_ERROR_STOP=1 --single-transaction -f /dev/stdin < migrations/016_drop_legacy_accounting_tables.sql
```

升级完成后：

```sh
go run ./cmd/asset-test
docker compose exec -T postgres psql -U rick -d reliable_chain -v ON_ERROR_STOP=1 < scripts/verify_mock_deposits.sql
docker compose exec -T postgres psql -U rick -d reliable_chain -v ON_ERROR_STOP=1 < scripts/verify_mock_credits.sql
```

预期：识别检查 11 组通过；入账检查 10 个用户通过；不平衡交易数和余额差异数为 0；平台托管与用户负债合计相等。
再次运行程序不重复入账。脚本假设这批测试用户没有其他资金流水或初始余额。
已有账本数据有外键和不可修改保护，不再直接清空 deposits 来重测；重复入账验证直接重跑，破坏性重置请使用独立测试库。

## 自动化检查

```sh
git diff --check
go test ./...
go vet ./...
```

集成测试需要设置 `TEST_DATABASE_URL` 为本地 PostgreSQL 测试库连接串，未设置时会跳过：

```sh
go test -race ./internal/asset -run '^Test(CreditDeposit|Accounting)' -v
```

各测试在独立 schema 中执行真实迁移并自动清理，不修改 public 下现有数据。
覆盖新库建表、旧数据转换及不一致迁移回滚、重新连接后重试、并发入账、各写入步骤失败回滚、可用与冻结账户记账，以及借贷不平、透支、错误用户/资产和直接篡改的拒绝。
可用转冻结仅在独立测试里使用 SQL 验证账本结构，没有新增冻结业务函数。
