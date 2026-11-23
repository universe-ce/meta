# Foundation / meta

模块路径保持 `github.com/universe-ce/meta/v3`。本库只提供小型技术模型和运行时适配，不承载各业务领域的共享实体。

| 包 | 职责 |
|---|---|
| `pkg/m` | 无第三方依赖的响应、错误、校验、分页、审计及审计请求模型 |
| `pkg/o` | 独立于 HTTP 框架的 OpenTelemetry 资源构造、上下文传播、flush 和 shutdown |
| `pkg/p` | 显式检查PostgreSQL ≥15后返回pgx/v5池；调用方拥有关闭权，不含业务模型或自动迁移 |
| `pkg/r` | 错误目录与 Iris 响应适配；公开类型别名指向 m |
| `pkg/mw` | Iris 鉴权、异常恢复、访问日志与审计适配 |
| `pkg/x` | JWT、密码、字符串、集合、分页、IP 和设备工具 |
| `pkg/t` | 已发布的审计 topic 和时间格式 |

新应用或领域代码需要技术 DTO 时可只导入 `pkg/m`。领域仍自行定义业务实体，不依赖 Iris、JWT 库或 OpenTelemetry 对象。

包名按用户要求统一为最短职责缩写：`r` = response、`x` = 工具、`t` = constant、`m` = model；中间件使用 `mw` = middleware，PostgreSQL 使用 `p`，OpenTelemetry 使用 `o`。`SwaggerDoc`、`NormalEditReq`、`InterfaceID` 位于纯模型包 `m`，HTTP 处理器位于 `mw`。升级工作树中的调用方及 Swagger 已同步。遇到重名时采用 `mr`、`mx` 等两字符别名。跨仓发行须发布包含 `m`、`mw` 等当前路径的 meta 版本，再锁定消费者和组合版本。

## service 与 meta 的分层边界

`p` 只构造和检查通用 pgx 连接池，`o` 只封装 OTel provider、传播、flush 和 shutdown。两包不读取服务配置，不执行数据库迁移，不决定业务事务、启动顺序或请求排空流程。

service 的 bootstrap 负责配置、创建、注入、停止接入、排空请求和关闭资源；Repository 负责业务 SQL、事务和所属模块迁移；common 负责自身公共业务模型及迁移。现有 UAA 的追踪配置与资源关闭仍由 `internal/bootstrap` 管理。当前 `p/o` 中没有需要迁往 service 的业务实现。

## 显式追踪生命周期

组合根在开始接收请求前创建 runtime，应用停止接收请求并完成在途请求后关闭 runtime：

```go
runtime, err := o.NewJaeger("service-name", collectorURL)
if err != nil {
    return err
}
app.Use(mw.TracingWithRuntime(runtime))
// 启动并运行服务；停止 ingress，等待在途请求完成。
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
return runtime.Shutdown(shutdownCtx)
```

`o.New`接收一个 exporter 并取得其所有权，便于组合根配置其他 exporter 和本地测试。`New`/`NewJaeger`不改进程全局 OTel 配置。`Start`从传入 context 和 carrier 提取 W3C/B3 父上下文及 baggage；由直接调用者结束 span。`TracingWithRuntime`负责 HTTP 请求的 span 结束，包括 panic 路径。

`ForceFlush`导出已结束的 span。`Shutdown`只启动一次共享清理，调用方 deadline 限制自己的等待；超时不会取消共享清理，后续调用继续等待真实结束，并收到相同最终结果。预先取消的 context 不启动清理。exporter 应遵守其接口的超时/关闭合同；应用不能将等待超时当作清理成功。没有 runtime 的 handler 仍保留访问日志和一次下游调用。

原 `TracingHandler(enable, service, collectorURL)` 继续可用，由调用方在HTTP drain后调用 `mw.ShutdownTracing(ctx)`。同一配置的手动生命周期入口复用runtime，保留原全局OTel接入。

需要兼容Iris自动停机的服务，在监听前使用 `app.Use(mw.TracingHandlerFor(app, enable, service, collectorURL))` 显式绑定。interrupt回调先排空这些application，再关闭它们的runtime；排空失败会保留资源并返回错误。绑定在启动阶段完成，首请求尚未进入middleware时也不会遗漏。CDKM沿用此入口；UAA已由自身bootstrap显式创建 `o.Runtime` 并通过 `TracingWithRuntime` 注入。

相同application/service/collector复用一个runtime；不同application，以及手动和自动生命周期，使用不同资源所有者，避免误关其他服务实例。自动停机不会关闭手动所有者的runtime。WebSocket等已劫持连接由服务自身关闭。新服务应使用显式runtime，避免全局配置与其他服务实例互相影响。

## PostgreSQL连接与所有权

在组合根创建池，通过本服务的outbound adapter注入用例：

```go
pool, err := p.NewPool(ctx, p.Options{
    DSN: localServiceDSN, MaxConns: 4, StartupTimeout: 5*time.Second,
})
if err != nil {
    return err
}
// 服务运行；停止接收请求并排空工作后：
pool.Close()
```

DSN使用pgx URL/keyword语法，显式给出host、port、database、user及TLS策略；本地开发示例见Compose `localdev/README.md`。MaxConns默认4、MinConns默认0，整个启动版本检查默认5秒并服从更短context。无init/global连接；解析失败不泄露原DSN。SQL、应用事务、migrations与ORM模型留在各服务，池的Close等待借出的连接归还。

旧GORM服务逐切片保留命名、hooks、软删除、NULL/时间、错误与事务语义；本包不自动替换其datasource，也不同时初始化两个默认连接池。公共包完整回归之外，真实PG测试需`-tags=integration`及隔离的`UCE_TEST_POSTGRES_DSN`；本地Compose的`test_meta.py`负责注入并执行。

## 兼容合同

`r.JsonResult`、`r.ApiError`、`r.ValidationError`、`x.PageInfo[T]`、`x.AuditLog` 和审计枚举使用类型/常量别名，可与 `m` 类型直接赋值，JSON 字段、业务码与 HTTP 行为保持。审计请求类型也由 `pkg/m` 声明；`InterfaceID` 保留原数字截断和字符串解析语义。不依赖反射名称或 gob 自动类型名作为对外协议。

以下历史语义由特征测试冻结，并没有在结构迁移时重定义：

- 错误目录的重复业务码、原始文案，`ProcErr`对传入指针的修改，鉴权与异常恢复的既有 HTTP 写头顺序。
- `GenerateJwtToken`的 HS256、issuer、字段及绝对秒 `expires_in`；旧 `GenerateToken`签名参数的错误行为。
- 历史 AES 生产/解包函数的字节结果及错误边界；存量密文升级需另外定义版本与迁移策略。
- 分页 nil/空列表行为、未复制 `HasNextPage`的旧语义，以及字符串和反射集合工具的边界。
- 审计仅把 HTTP 200 作为成功，枚举/日期/topic 不变，消息参数在异步发送前完成构造。

新代码可用 `m.ApiError.WithCause`复制错误模板；`r.ProcErr`保留原有可变行为，不可把共享全局模板交给多个 goroutine 并发修改。函数及类型的兼容行为由既有特征测试验证，包路径调整需同步所有消费者。

## 本地回归

使用工作区锁定的 Go 1.27.4：

```sh
GOTOOLCHAIN=local GOWORK=off go test -tags=http2legacy -race -count=1 ./...
GOTOOLCHAIN=local go test -tags=http2legacy -race -count=1 ./...
```

第二条应在包含本地各升级模块的 workspace 中运行。公共包变更还须编译所有实际消费者，并核对它们解析到本地 meta 源码，而不是已发布旧版本。`pkg/m`的依赖闭包只允许标准库及本包。

测试含旧错误目录、完整 JSON/JWT/密文样例、Iris 真实处理链、鉴权/异常/审计回归、并发快照与追踪生命周期。全部网络交互使用本地测试服务，不需要生产配置。
