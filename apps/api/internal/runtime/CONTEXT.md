# Platform Runtime Context

`internal/runtime` 是 IdeaBoard API 进程的 **Platform Runtime Context**。它不是业务上下文，但拥有自身的事实（Facts）、显式生命周期状态机（Lifecycle State Machine）、所有权规则与架构不变式。本模块采用 DDD 限界与 Clean/Hexagonal Architecture 组织，严禁沦为杂乱的接线（wiring）包。

---

## 1. 显式生命周期状态机 (Lifecycle State Machine)

```text
created -> initializing -> recovering -> ready -> draining -> stopped
```

### 核心不变式 (Required Invariants)
- **`ready`**: 必须依赖的数据库、River、NATS 及适配器全部健康，方可标记就绪并开放流量。
- **`recovering`**: 正在收敛前次未完成的 running 任务与清理遗留 River Jobs；**硬性门禁：此阶段严禁接受新任务执行**。
- **`draining`**: 捕获停机信号，不再接受新工作，并在固定关机预算（Shutdown Budget）内优雅关闭各 Runner。
- **`stopped`**: 所有 Runtime 持有的连接池、底层资源与 Closer 均已安全释放。

---

## 2. 概念与通用语言 (Ubiquitous Language)

- **Domain (`domain/`)**: 运行时领域内核，拥有生命周期状态机（`StateMachine`）、状态转移规则、组件健康事实（`ComponentStatus`）与 Readiness 评估规则。
- **Ports (`ports/`)**: 运行时接缝契约，定义组件生命周期 `Runner`、就绪检查 `HealthChecker`、启动恢复协调器 `StartupRecoveryCoordinator` 与资源清理接口 `ResourceCloser`。
- **Application (`application/`)**:
  - `lifecycle/`: 状态机驱动的生命周期协调（`Coordinator`），管理停机预算与排空（Drain）。
  - `orchestration/`: 依赖图启动计划（`ExecuteStartup`）与任务启动恢复（`TaskRecoveryCoordinator`）。
- **Composition Root (`composition/`)**: 全进程唯一的依赖装配根与 CLI 命令调度器，负责配置加载、数据库池与服务装配，并构建出受生命周期管控的 `App`。
- **Interfaces (`interfaces/`)**: 传输与执行适配层：
  - `http/`: HTTP REST API 传输适配器，路由注册、Session/Auth 鉴权中间件、速率限制与静态资源代理。
  - `worker/`: River 队列消费、外部 AI Provider 执行适配器与后台定时对账任务。
- **Adapters (`adapters/`)**: 平台运行时辅助适配器：
  - `billingregistry/`: Workflow 能力到 Billing 计费项定义的运行时装配映射。
  - `webdist/`: 编译嵌入前端 SPA 产物的静态资源适配器。

---

## 3. 目录结构

```text
internal/runtime/
├── CONTEXT.md                    # 本文档
├── architecture_test.go          # 运行时分层依赖与适配器隔离架构测试
├── domain/                       # 运行时状态、组件事实、Readiness 评估
├── ports/                        # Runner、健康检查、恢复协调契约接口
├── application/
│   ├── lifecycle/                # 状态机协调器、停机预算管理与排空
│   └── orchestration/            # 依赖图编排、有序启动恢复计划
├── composition/                  # 唯一的依赖装配根 (Composition Root) 与 CLI 入口
├── infrastructure/               # PostgreSQL/River、NATS、HTTP 与 config adapters
│   ├── config/
│   ├── database/
│   ├── http/
│   └── nats/
├── interfaces/
│   ├── http/                     # HTTP 路由与传输适配器 (原 httpapi)
│   └── worker/                   # 根层门面 (worker.go) 与执行适配器
│       ├── billing/              # 积分对账与支付后台定时任务
│       ├── execution/            # 任务执行编排与租约生命周期状态机
│       ├── providers/            # 外部 AI Provider 适配器
│       └── river/                # River 队列工作者与终态失败处理
└── adapters/
    ├── billingregistry/          # Workflow 与 Billing 映射适配
    └── webdist/                  # 前端 SPA 静态产物 embed
```

---

## 4. 架构硬性约束 (Hard Invariants)

1. **严格单向依赖 (Strict One-Way Dependency)**:
   - `domain` 处于最内核，严禁依赖任何外部 runtime 层或业务上下文；
   - `ports` 仅依赖 `domain`；
   - `application` 仅依赖 `domain` 与 `ports`；
   - `composition` 作为装配根，组装各层与业务上下文；
   - **严禁任何业务上下文反向引入 `internal/runtime`**。
2. **传输与执行接口互斥隔离 (Interface Isolation)**:
   - `interfaces/http` 与 `interfaces/worker` 互不依赖、互不相识；
   - 二者仅通过持久化事实源（PostgreSQL）与事件总线（NATS JetStream）间接协同；
   - 接口层严禁反向依赖 `composition`。
3. **Worker 内部子包解耦与单向约束 (Worker Subpackage Boundaries)**:
   - `execution` 与 `providers` 之间严禁互相 import，统一基于 `runtime/ports` 契约解耦；
   - `billing` 仅处理后台对账与支付定时任务，严禁反向依赖 `execution` 或 `providers`；
   - `river` 仅作为队列装配适配器依赖 `execution` 与 `providers`；
   - `worker` 根目录仅保留统一公开门面（`worker.go`）与跨领域端到端契约测试。
4. **唯一的装配根 (Sole Composition Root)**:
   - `composition/` 是进程内唯一的对象构造与依赖组装中心，业务包与其他 runtime 子包不持有全局 wiring。
5. **根目录无实现代码**:
   - `internal/runtime/` 根目录仅保留上下文契约与架构测试。
6. **任务提交与队列事务性一致**:
   - 业务 Task 提交与 River Job 入队必须在同一 PostgreSQL 事务中原子完成。
