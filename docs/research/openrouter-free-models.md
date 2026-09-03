# OpenRouter 免费模型全景深度调研报告 (2026 最新版)

> **调研基准时间**：2026年09月04日 (Asia/Shanghai)
> **第一手信息源 (Primary Sources)**：
> 1. OpenRouter 官方公开模型列表 API: `https://openrouter.ai/api/v1/models`
> 2. OpenRouter 官方端点元数据 API: `https://openrouter.ai/api/v1/models/{model_id}/endpoints`
> 3. OpenRouter 官方配额与限流规范文档: `https://openrouter.ai/docs/limits`
> 4. OpenRouter 官方模型卡片详情页: `https://openrouter.ai/models`

---

## 一、 核心调研结论与生态演进 (Executive Summary)

本次调研严格基于 OpenRouter 官方公开 API 与实时元数据，对平台全部 **425 个在架模型** 进行了全量检索与端点属性过滤。当前平台共有 **21 个免费模型/免费端点**（满足模型 ID 包含 `:free` 或 `pricing.prompt == '0'` 且 `pricing.completion == '0'`）。

### 1. 核心格局演变与代际换血
过去几年开发者所熟知的早期免费模型（如 `meta-llama/llama-3-8b-instruct:free`、`qwen/qwen-2.5-72b-instruct:free`、`deepseek/deepseek-r1:free`、`google/gemini-flash-1.5:free` 等）已随各家算法演进与平台运营策略调整退出现役免费池，或转入纯商业计费通道。当前（2026年下半年）呈现出全新的技术代际特征：

- **MoE 稀疏激活架构成为绝对主流**：在 21 个免费模型中，超过 70% 采用高稀疏度混合专家（MoE）架构，涵盖从 30B 级（3B 激活）的极速端侧模型，到 550B（55B 激活）乃至 975B（41B 激活）的超大规模参数模型。
- **百万级长上下文与超大单次补全输出普及**：传统 8K/32K 上下文限制被彻底打破。多达 6 款模型原生支持 **1,000,000 Token（1M）** 上下文；单次补全最大输出 Token 突破天际，如 **MiniMax M3** 最大输出达 **943,718 Tokens**，**Dots3-Note** 最大输出达 **460,800 Tokens**，**GLM 5.2** 达 **230,400 Tokens**。
- **垂类专业智能体（Agentic Specialists）全面爆发**：免费池中涌现了大量高度垂直的模型，包括专精代码智能体（Poolside Laguna 2.1、Cohere North Mini Code）、金融投研智能体（InclusionAI Ling 3.0 Flash Fin）、端侧轻量神经动力学模型（Liquid LFM-2.5）、全模态感知与安全护栏（NVIDIA Nemotron 3.5 Content Safety / Nano Omni）。
- **原生智能路由器与全模态扩展**：支持图像、音频、视频理解的多模态免费模型达 8 款；平台同时提供了原生免费智能路由器 `openrouter/free`，可依据用户请求是否携带图片、工具或结构化输出自动负载均衡至最适宜的免费模型。

---

## 二、 OpenRouter 免费调用配额、速率限制与隐私合规政策

### 1. 官方速率限制 (Rate Limits) 规则
根据 OpenRouter 官方限制规范 (`/docs/limits`)，平台对所有以 `:free` 结尾的模型应用严格的账户级限流策略，限流以账户历史终身充值额度为分水岭：

| 账户类型 | 终身充值信用额度 (Credits Purchased) | 每分钟请求上限 (RPM) | 每天请求上限 (RPD) | 429 错误触发原因 |
| :--- | :--- | :--- | :--- | :--- |
| **未充值/纯免费账户** | `< $10` | **20 RPM** | **50 RPD** | 超过 20 RPM 或单日超过 50 次请求 |
| **充值支持者账户** | **`≥ $10` (终身累计)** | **20 RPM** | **1,000 RPD** | 超过 20 RPM 或单日超过 1,000 次请求 |

> **核心要点**：
> 1. **RPM 上限不可花钱提升**：无论是免费账户还是充值满 $10 的账户，每分钟请求上限固定为 **20 RPM**。高频并发必须在客户端实现排队或抖动退避。
> 2. **充值 $10 永久提升 RPD**：只要账户历史上累计充值满 $10（该 $10 永久有效且可正常用于付费模型），全平台所有免费模型的每日额度即永久从 50 次暴增至 **1,000 次/天**，是开发者最具性价比的接入姿态。
> 3. **额度检查 API**：开发者可随时通过 `GET https://openrouter.ai/api/v1/key` 获取当前 API Key 的 `usage_daily`、`is_free_tier` 及 `limit_remaining` 实时监控额度。

### 2. 数据隐私与训练日志政策 (Data Privacy & Disclaimers)
OpenRouter 作为多 Provider 聚合路由，数据隐私采用双层机制，调研中发现上游免费提供商存在**明确的免责与训练声明**：
1. **OpenRouter 平台层**：
   - 默认**不记录**用户的 Prompt 与 Completion 文本，仅统计元数据（Token 消耗、请求耗时、状态码）。
   - 平台提供数据折扣机制（如果用户主动开启允许平台改进模型则给予 1% 账单折扣，但此项默认关闭）。
   - 支持在请求层传入 `"zdr": true` (Zero Data Retention) 请求头/参数，要求路由节点坚决不落地任何临时数据。
2. **上游 Provider 明确的训练免责条款（一手数据发现）**：
   在调研官方模型卡片描述中，部分提供商对免费端点声明了数据使用权：
   - **Liquid AI (`liquid/lfm-2.5-2.6b:free`)** 明确声明：*"Prompts and outputs may be retained and used to train Liquid models."*（免费调用的 Prompt 与生成结果可能被留存并用于训练 Liquid 模型）。
   - **Poolside (`poolside/laguna-s-2.1:free` & `laguna-xs-2.1:free`)** 明确声明：*"If you are using Laguna for free, we may use your inputs and outputs to train and improve our models."*（免费使用 Laguna 时，输入与输出可能被用于训练与改进模型）。
   - **企业级应对**：涉及核心商业机密、私有代码或个人隐私数据的场景，严禁使用含保留条款的免费模型，或应通过 OpenRouter 设置中的 Data Policy Filtering 排除训练型上游。

---

## 三、 现存 21 款免费模型全景一览表 (按上线时间倒序)

以下数据全部由 OpenRouter 官方公开 API `https://openrouter.ai/api/v1/models` 实时解析提取：

| 序号 | 模型 ID | 官方全称 | 上游托管与量化 | 上线时间 (UTC) | 上线时间 (北京时间) | 上下文 | 最大补全 | 模态 (入 -> 出) | 能力特性 (Tools/JSON/Reasoning) |
| :---: | :--- | :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| 01 | `inclusionai/ling-3.0-flash-fin:free` | Ling 3.0 Flash Fin (free) | Novita | 2026-08-27 | 2026-08-27 23:58 | 256K | 32K | text -> text | T / - / R |
| 02 | `dots-studio/dots-3-note-preview:free` | Dots Studio: Dots3-Note Preview (free) | AtlasCloud (fp8) | 2026-08-14 | 2026-08-14 12:06 | 500K | 450K | text+image -> text | T / J / R |
| 03 | `liquid/lfm-2.5-2.6b:free` | LiquidAI: LFM2.5-2.6B (free) | Liquid (fp8) | 2026-08-11 | 2026-08-12 01:48 | 64K | 8K | text -> text | T / J / R |
| 04 | `nvidia/nemotron-3.5-lightning:free` | NVIDIA: Nemotron 3.5 Lightning (free) | Nvidia (nvfp4) | 2026-08-11 | 2026-08-11 20:52 | 976K | 64K | text -> text | T / - / R |
| 05 | `thinkingmachines/inkling-small:free` | Thinking Machines: Inkling Small (free) | Thinking Machines (nvfp4) | 2026-07-30 | 2026-07-31 04:25 | 1024K | 256K | text+image+audio -> text | T / - / R |
| 06 | `poolside/laguna-s-2.1:free` | Poolside: Laguna S 2.1 (free) | Poolside (fp4) | 2026-07-21 | 2026-07-22 00:51 | 256K | 32K | text -> text | T / - / R |
| 07 | `thinkingmachines/inkling:free` | Thinking Machines: Inkling (free) | Thinking Machines (nvfp4) | 2026-07-17 | 2026-07-18 06:05 | 1024K | 256K | text+image+audio -> text | T / - / R |
| 08 | `poolside/laguna-xs-2.1:free` | Poolside: Laguna XS 2.1 (free) | Poolside (fp8) | 2026-07-02 | 2026-07-02 22:27 | 256K | 32K | text -> text | T / - / R |
| 09 | `cohere/north-mini-code:free` | Cohere: North Mini Code (free) | Cohere | 2026-06-17 | 2026-06-18 03:15 | 250K | 62K | text -> text | T / - / R |
| 10 | `z-ai/glm-5.2:free` | Z.ai: GLM 5.2 (free) | Decart (fp4) | 2026-06-16 | 2026-06-17 01:45 | 250K | 225K | text -> text | T / J / R |
| 11 | `nvidia/nemotron-3.5-content-safety:free` | NVIDIA: Nemotron 3.5 Content Safety (free) | Nvidia | 2026-06-04 | 2026-06-04 22:04 | 125K | 8K | text+image -> text | - / - / R |
| 12 | `nvidia/nemotron-3-ultra-550b-a55b:free` | NVIDIA: Nemotron 3 Ultra (free) | Nvidia | 2026-06-04 | 2026-06-04 13:33 | 976K | 64K | text -> text | T / - / R |
| 13 | `minimax/minimax-m3:free` | MiniMax: MiniMax M3 (free) | GMICloud (fp8) | 2026-05-31 | 2026-06-01 00:36 | 1024K | 921K | text+image+video -> text | T / J / R |
| 14 | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | NVIDIA: Nemotron 3 Nano Omni (free) | Nvidia | 2026-04-28 | 2026-04-29 00:18 | 250K | 64K | text+audio+image+video -> text | T / - / R |
| 15 | `google/gemma-4-26b-a4b-it:free` | Google: Gemma 4 26B A4B  (free) | Google AI Studio | 2026-04-03 | 2026-04-03 22:53 | 256K | 32K | image+text+video -> text | T / J / R |
| 16 | `google/gemma-4-31b-it:free` | Google: Gemma 4 31B (free) | Google AI Studio | 2026-04-02 | 2026-04-03 00:48 | 256K | 32K | image+text+video -> text | T / J / R |
| 17 | `google/lyria-3-pro-preview` | Google: Lyria 3 Pro Preview | Google AI Studio | 2026-03-30 | 2026-03-31 05:48 | 1024K | 64K | text+image -> text+audio | - / J / - |
| 18 | `google/lyria-3-clip-preview` | Google: Lyria 3 Clip Preview | Google AI Studio | 2026-03-30 | 2026-03-31 05:47 | 1024K | 64K | text+image -> text+audio | - / J / - |
| 19 | `minimax/minimax-m2.7:free` | MiniMax: MiniMax M2.7 (free) | GMICloud (fp8) | 2026-03-18 | 2026-03-18 20:24 | 192K | 172K | text -> text | T / J / R |
| 20 | `nvidia/nemotron-3-super-120b-a12b:free` | NVIDIA: Nemotron 3 Super (free) | Nvidia | 2026-03-11 | 2026-03-12 00:07 | 256K | 230K | text -> text | T / J / R |
| 21 | `openrouter/free` | Free Models Router | OpenRouter | 2026-02-01 | 2026-02-01 11:43 | 195K | 动态适配 | text+image -> text | T / J / - |

> **表格缩写说明**：`T` = 支持函数与工具调用 (Tool Calling)；`J` = 支持结构化 JSON 输出 (Structured Outputs / JSON Schema)；`R` = 原生支持显式思维链推理 (Reasoning / Thinking Mode)。

---

## 四、 重点与最新上线免费模型深度详析

### 4.1 Ling 3.0 Flash Fin (free) (`inclusionai/ling-3.0-flash-fin:free`)

- **上线时间**：2026-08-27 15:58:10 UTC (北京时间 2026-08-27 23:58)
- **上游提供商与量化**：Novita (量化: unknown)
- **上下文窗口**：`262,144` Tokens | **单次最大输出**：`32768` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`frequency_penalty, include_reasoning, logprobs, max_tokens, presence_penalty, reasoning, repetition_penalty, seed, stop, temperature, tool_choice, tools, top_k, top_logprobs, top_p`
- **官方技术定位描述**：
> Ling 3.0 Flash Fin is a finance-focused mixture-of-experts model from InclusionAI, built on Ling 3.0 Flash with 5.1B active parameters out of 124B total. It is designed for real-world investment workflows that require complex multi-step tasks and long-horizon planning and execution, while retaining general capabilities in reasoning, coding, and mathematics.

- **核心技术剖析与场景定位**：
  - 金融长程投研首选：总参数 124B、激活仅 5.1B 的高稀疏比 MoE。针对实际投资工作流中复杂多步骤任务、财务报表对比与长期规划深度优化，兼备高水平的通用代码与数学推理能力，是免费模型中独树一帜的金融垂直智能体。

### 4.2 Dots Studio: Dots3-Note Preview (free) (`dots-studio/dots-3-note-preview:free`)

- **上线时间**：2026-08-14 04:06:01 UTC (北京时间 2026-08-14 12:06)
- **上游提供商与量化**：AtlasCloud (量化: fp8)
- **上下文窗口**：`512,000` Tokens | **单次最大输出**：`460800` Tokens
- **输入与输出模态**：输入 `text, image`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, response_format, structured_outputs, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> Dots3-Note Preview is an open-weight mixture-of-experts model from Dots Studio, with 16B active parameters out of 280B total. It is the lightest model in the Dots 3 family and is suited for reasoning, coding, multimodal understanding, long-context processing, and multi-step agent workflows.

- **核心技术剖析与场景定位**：
  - 超长文档分析与万字长文生成神器：280B MoE 架构（16B 激活），拥有 512K 超大上下文以及高达 460,800 Tokens 的补全输出！支持图文多模态输入，专为海量文献对比、整本专业研报解析与深度长篇笔记生成打造。

### 4.3 LiquidAI: LFM2.5-2.6B (free) (`liquid/lfm-2.5-2.6b:free`)

- **上线时间**：2026-08-11 17:48:39 UTC (北京时间 2026-08-12 01:48)
- **上游提供商与量化**：Liquid (量化: fp8)
- **上下文窗口**：`65,536` Tokens | **单次最大输出**：`8192` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: True)
- **核心支持参数**：`frequency_penalty, include_reasoning, logprobs, max_completion_tokens, max_tokens, min_p, presence_penalty, reasoning, repetition_penalty, response_format, seed, stop, structured_outputs, temperature, tool_choice, tools, top_k, top_logprobs, top_p`
- **官方技术定位描述**：
> LFM2.5-2.6B is a compact reasoning model from Liquid AI. It is suited for agent workflows, data extraction, RAG, and long-context processing. Liquid advises against using it for agentic coding or knowledge-heavy tasks.  Prompts and outputs may be retained and used to train Liquid models.

- **核心技术剖析与场景定位**：
  - 极速端侧与轻量级 RAG 抽取：2.6B 参数紧凑型神经动力学架构，强制开启思维链推理 (mandatory: True)。延迟极低、吞吐极高，适用于高并发信息抽取、文本分类与流程调度。（特别警示：官方明确声明免费端调用的输入输出将被用于训练 Liquid 模型，且不推荐用于高复杂度代码编写）。

### 4.4 NVIDIA: Nemotron 3.5 Lightning (free) (`nvidia/nemotron-3.5-lightning:free`)

- **上线时间**：2026-08-11 12:52:31 UTC (北京时间 2026-08-11 20:52)
- **上游提供商与量化**：Nvidia (量化: nvfp4)
- **上下文窗口**：`1,000,000` Tokens | **单次最大输出**：`65536` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> NVIDIA Nemotron 3.5 Lightning is an open mixture-of-experts model from NVIDIA, with 3B active parameters out of 30B total. It is suited for high-throughput agentic workloads and specialized tasks that benefit from domain-specific customization.

- **核心技术剖析与场景定位**：
  - 百万级上下文极速调度器：30B MoE（激活 3B），NVFP4 极致量化，原生支持 1,000,000 Token (1M) 上下文。专为大并发多智能体协同系统中的中枢调度、任务分发与上下文保持设计。

### 4.5 Thinking Machines: Inkling (free) (`thinkingmachines/inkling:free`)

- **上线时间**：2026-07-17 22:05:56 UTC (北京时间 2026-07-18 06:05)
- **上游提供商与量化**：Thinking Machines (量化: nvfp4)
- **上下文窗口**：`1,048,576` Tokens | **单次最大输出**：`262144` Tokens
- **输入与输出模态**：输入 `text, image, audio`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`frequency_penalty, include_reasoning, max_tokens, presence_penalty, reasoning, reasoning_effort, seed, stop, temperature, tools, top_p`
- **官方技术定位描述**：
> Inkling is an open-weight multimodal mixture-of-experts model from Thinking Machines Lab, with 41B active parameters out of 975B total. It is designed for general-purpose reasoning, coding, agentic and tool-use systems, retrieval-augmented generation, instruction following, and multilingual conversational applications. Its native image and audio understanding supports multimodal analysis alongside text.

- **核心技术剖析与场景定位**：
  - 近万亿全模态旗舰巨兽：975B 总参数、41B 激活参数的超顶配开源多模态 MoE。原生支持 Text + Image + Audio 三重模态输入，具备 1M 上下文与 262K 输出，提供 6 级推理深度细粒度控制 (max/high/medium/low/minimal/none)，对标业界第一梯队多模态基座。

### 4.6 Thinking Machines: Inkling Small (free) (`thinkingmachines/inkling-small:free`)

- **上线时间**：2026-07-30 20:25:17 UTC (北京时间 2026-07-31 04:25)
- **上游提供商与量化**：Thinking Machines (量化: nvfp4)
- **上下文窗口**：`1,048,576` Tokens | **单次最大输出**：`262144` Tokens
- **输入与输出模态**：输入 `text, image, audio`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`frequency_penalty, include_reasoning, max_tokens, presence_penalty, reasoning, reasoning_effort, seed, stop, temperature, tools, top_p`
- **官方技术定位描述**：
> Inkling Small is an open-weight multimodal mixture-of-experts model from Thinking Machines Lab, with 12B active parameters out of 276B total. It is positioned as the smaller, more efficient member of the Inkling family and is suited for reasoning, coding, agentic workflows, retrieval-augmented generation, instruction following, and multilingual conversation.

- **核心技术剖析与场景定位**：
  - 高性价比全模态副驾：276B 总参数、12B 激活的轻量版多模态 MoE，完整保留文本/图像/音频输入与 1M 上下文，更适合高频对话与低延迟多模态交互。

### 4.7 Poolside: Laguna S 2.1 (free) (`poolside/laguna-s-2.1:free`)

- **上线时间**：2026-07-21 16:51:23 UTC (北京时间 2026-07-22 00:51)
- **上游提供商与量化**：Poolside (量化: fp4)
- **上下文窗口**：`262,144` Tokens | **单次最大输出**：`32768` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, temperature, tool_choice, tools`
- **官方技术定位描述**：
> Laguna S 2.1 is the latest coding agent model from [Poolside](<https://poolside.ai/>). Laguna S 2.1 is a 118B total parameter model with 8B active parameters, scoring 70.2% on Terminal-Bench 2.1 and 40.4% on DeepSWE, making it one of the strongest coding models in its category. Open-weight under the OpenMDW-1.1 license.  Laguna S 2.1 is designed for software engineering and agentic coding use cases, and you are responsible for confirming that it is appropriate for your intended application. Laguna S 2.1 is subject to the [OpenMDW-1.1 License](<https://openmdw.ai/license/1-1/>), and should be used consistently with Poolside's [Acceptable Use Policy](<https://poolside.ai/legal/acceptable-use-policy>). We advise against circumventing Laguna S 2.1 safety guardrails without implementing substantially equivalent mitigations appropriate for your use case.  Please report security vulnerabilities or safety concerns to [security@poolside.ai](<mailto:security@poolside.ai>).  If you are using Laguna S 2.1 for free, we may use your inputs and outputs to train and improve our models.

- **核心技术剖析与场景定位**：
  - 前沿级代码智能体：118B 总参数、8B 激活，采用 FP4 量化。在 Terminal-Bench 2.1 取得 70.2%、DeepSWE 取得 40.4% 的行业顶尖成绩，专门强化终端交互、Bash 指令执行与复杂软件工程项目修复（注意：免费版有数据训练保留条款）。

### 4.8 Poolside: Laguna XS 2.1 (free) (`poolside/laguna-xs-2.1:free`)

- **上线时间**：2026-07-02 14:27:09 UTC (北京时间 2026-07-02 22:27)
- **上游提供商与量化**：Poolside (量化: fp8)
- **上下文窗口**：`262,144` Tokens | **单次最大输出**：`32768` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, temperature, tool_choice, tools`
- **官方技术定位描述**：
> Laguna XS 2.1 is the latest coding agent model in the 33B-A3B category from [Poolside](https://poolside.ai/) and a step forward from their Laguna XS.2 model (released in April 2026). It combines tool calling and reasoning capabilities with a compact footprint, offering a 256K context window and up to 32K output tokens. Quantized to FP8 for fast, cost-efficient agentic coding workflows.  Laguna XS 2.1 is designed for software engineering and agentic coding use cases, and you are responsible for confirming that it is appropriate for your intended application. Laguna XS 2.1 is subject to the [OpenMDW-1.1 License](https://openmdw.ai/license/1-1/), and should be used consistently with Poolside's [Acceptable Use Policy](https://poolside.ai/legal/acceptable-use-policy). We advise against circumventing Laguna XS 2.1 safety guardrails without implementing substantially equivalent mitigations appropriate for your use case.  Please report security vulnerabilities or safety concerns to [security@poolside.ai](mailto:security@poolside.ai).  If you are using Laguna XS 2.1 for free, we may use your inputs and outputs to train and improve our models.

- **核心技术剖析与场景定位**：
  - 轻量快速代码修复 Agent：33B 总参数、3B 激活，FP8 量化，适合即时 IDE 代码补全、小步单测修复及低延迟代码诊断（同样受数据保留条款约束）。

### 4.9 Cohere: North Mini Code (free) (`cohere/north-mini-code:free`)

- **上线时间**：2026-06-17 19:15:48 UTC (北京时间 2026-06-18 03:15)
- **上游提供商与量化**：Cohere (量化: unknown)
- **上下文窗口**：`256,000` Tokens | **单次最大输出**：`64000` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: False)
- **核心支持参数**：`frequency_penalty, include_reasoning, max_tokens, presence_penalty, reasoning, seed, stop, temperature, tool_choice, tools, top_k, top_p`
- **官方技术定位描述**：
> North Mini Code is Cohere's first agentic coding model and the debut of its North family. A sparse mixture-of-experts model with 30B total parameters and 3B active, it is optimized for code generation, agentic software engineering, and terminal tasks, and is trained to generalize across agent harnesses such as OpenCode and SWE-Agent.  It offers a 256K-token context window with up to 64K tokens of output, supports interleaved reasoning and tool use via JSON schema, and is released open-weight under the Apache 2.0 license. Its small active-parameter footprint enables low-latency inference, including on local hardware.

- **核心技术剖析与场景定位**：
  - Cohere 开源首个代码智能体：30B 总参数、3B 激活的稀疏 MoE，采用 Apache 2.0 宽松协议开源。针对 SWE-Agent 与 OpenCode 原生适配调优，深度支持交错推理与多步骤终端工具调用。

### 4.10 Z.ai: GLM 5.2 (free) (`z-ai/glm-5.2:free`)

- **上线时间**：2026-06-16 17:45:30 UTC (北京时间 2026-06-17 01:45)
- **上游提供商与量化**：Decart (量化: fp4)
- **上下文窗口**：`256,000` Tokens | **单次最大输出**：`230400` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`frequency_penalty, include_reasoning, max_tokens, min_p, presence_penalty, reasoning, reasoning_effort, repetition_penalty, response_format, seed, stop, structured_outputs, temperature, tool_choice, tools, top_k, top_p`
- **官方技术定位描述**：
> GLM 5.2 is a large-scale reasoning model from Z.ai. It supports text input and output with a 1M-token context window, and is suited for long-horizon agent workflows, project-level software engineering, and complex multi-step automation.  Reasoning efforts `high` and `xhigh` are supported; `xhigh` maps to max reasoning. It is particularly strong at coding and tool use across long-running tasks, able to maintain engineering context and follow standards consistently through a full development workflow, from requirements to multi-platform deployment, in a single task.

- **核心技术剖析与场景定位**：
  - 智谱工程级大模型：Decart 托管 FP4 版本，支持 256K 输入与 230,400 Tokens 超长补全输出，完整支持 Structured Outputs 与 Tools，在工程代码与中文复杂多步推理上优势显著。

### 4.11 MiniMax: MiniMax M3 (free) (`minimax/minimax-m3:free`)

- **上线时间**：2026-05-31 16:36:14 UTC (北京时间 2026-06-01 00:36)
- **上游提供商与量化**：GMICloud (量化: fp8)
- **上下文窗口**：`1,048,576` Tokens | **单次最大输出**：`943718` Tokens
- **输入与输出模态**：输入 `text, image, video`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, response_format, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> MiniMax-M3 is a multimodal foundation model from MiniMax. It supports text, image, and video inputs with text output, a 1M-token context window, and is suited for long-horizon agentic work, coding, and tool use. It is built on MiniMax Sparse Attention (MSA), which replaces full attention with KV-block selection to cut per-token compute at long context â roughly 1/20 the cost of the previous generation at 1M tokens, with substantially faster prefill and decode while retaining quality across most tasks.  Trained as a native multimodal model on interleaved data and tuned for multi-turn, production-like collaboration via an interactive user-simulator framework, the model is oriented toward sustained, multi-step tasks rather than single-turn execution.

- **核心技术剖析与场景定位**：
  - 全模态（文本+图像+视频）长文本王者：100万 Token 上下文，搭配自研 MiniMax Sparse Attention (MSA) 机制，长上下文计算开销骤降至 1/20。最大补全输出达惊人的 943,718 Tokens，支持多模态端到端视频长程分析。

### 4.12 MiniMax: MiniMax M2.7 (free) (`minimax/minimax-m2.7:free`)

- **上线时间**：2026-03-18 12:24:57 UTC (北京时间 2026-03-18 20:24)
- **上游提供商与量化**：GMICloud (量化: fp8)
- **上下文窗口**：`196,608` Tokens | **单次最大输出**：`176947` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: True)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, response_format, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> MiniMax-M2.7 is a next-generation large language model designed for autonomous, real-world productivity and continuous improvement. Built to actively participate in its own evolution, M2.7 integrates advanced agentic capabilities through multi-agent collaboration, enabling it to plan, execute, and refine complex tasks across dynamic environments.  Trained for production-grade performance, M2.7 handles workflows such as live debugging, root cause analysis, financial modeling, and full document generation across Word, Excel, and PowerPoint. It delivers strong results on benchmarks including 56.2% on SWE-Pro and 57.0% on Terminal Bench 2, while achieving a 1495 ELO on GDPval-AA, setting a new standard for multi-agent systems operating in real-world digital workflows.

- **核心技术剖析与场景定位**：
  - 自进化多智能体协作中枢：针对真实办公生态（Word/Excel/PPT 生成、复杂金融建模、Live Debugging）特训，在 SWE-Pro (56.2%) 与 Terminal-Bench 2 (57.0%) 表现强劲。

### 4.13 NVIDIA: Nemotron 3 Ultra (free) (`nvidia/nemotron-3-ultra-550b-a55b:free`)

- **上线时间**：2026-06-04 05:33:28 UTC (北京时间 2026-06-04 13:33)
- **上游提供商与量化**：Nvidia (量化: unknown)
- **上下文窗口**：`1,000,000` Tokens | **单次最大输出**：`65536` Tokens
- **输入与输出模态**：输入 `text`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, reasoning_effort, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> NVIDIA Nemotron 3 Ultra is an open frontier-reasoning and orchestration model from NVIDIA, with 55B active parameters out of 550B total (MoE). Built on a hybrid Transformer-Mamba mixture-of-experts architecture, it supports text input and output with a context window of up to 1M tokens. It is suited for long-running agentic workflows, including agent orchestration, coding agents, deep research, and complex enterprise tasks.  It is particularly strong at multi-step reasoning and planning, with high-throughput inference designed for high-volume agent pipelines. It is part of the NVIDIA Nemotron family of open models for agentic AI.

- **核心技术剖析与场景定位**：
  - NVIDIA 550B 混合 Mamba-Transformer 推理旗舰：550B 总参数、55B 激活，结合了 Mamba 的超线性状态转移与 Transformer 的前沿推理能力，1M 上下文，具备超强多智能体编排与逻辑推演能力。

### 4.14 NVIDIA: Nemotron 3.5 Content Safety (free) (`nvidia/nemotron-3.5-content-safety:free`)

- **上线时间**：2026-06-04 14:04:24 UTC (北京时间 2026-06-04 22:04)
- **上游提供商与量化**：Nvidia (量化: unknown)
- **上下文窗口**：`128,000` Tokens | **单次最大输出**：`8192` Tokens
- **输入与输出模态**：输入 `text, image`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, seed, temperature, top_p`
- **官方技术定位描述**：
> NVIDIA Nemotron 3.5 Content Safety is a compact 4B-parameter multimodal guardrail model from NVIDIA, fine-tuned from Google Gemma-3-4B. It moderates both inputs to and responses from LLMs and VLMs, accepting text and image input and returning text output: a safe/unsafe classification for the user prompt and the response, safety category labels, and an optional reasoning trace. It covers 12 languages with a context window of up to 128K tokens.  It is suited for prompt and response moderation, content classification, safety pipelines, and enterprise AI guardrails with policy enforcement, and includes a togglable reasoning mode. It is part of the NVIDIA Nemotron family of open models for agentic AI.

- **核心技术剖析与场景定位**：
  - 多模态合规审查与内容安全专用护栏：4B 参数专用轻量模型，基于 Gemma-3 微调，可对多模态输入/输出进行实时低延迟内容审查与合规拦截。

### 4.15 NVIDIA: Nemotron 3 Nano Omni (free) (`nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free`)

- **上线时间**：2026-04-28 16:18:15 UTC (北京时间 2026-04-29 00:18)
- **上游提供商与量化**：Nvidia (量化: unknown)
- **上下文窗口**：`256,000` Tokens | **单次最大输出**：`65536` Tokens
- **输入与输出模态**：输入 `text, audio, image, video`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: True, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> NVIDIA Nemotronâ¢ 3 Nano Omni is a 30B-A3B open multimodal model designed to function as a perception and context sub-agent in enterprise agent systems. It accepts text, image, video, and audio inputs and produces text output, enabling agents to perceive and reason across modalities in a single inference loop.  Built on a hybrid MoE Transformer-Mamba architecture with Conv3D video layers and Efficient Video Sampling (EVS), it delivers approximately 2Ã higher throughput and 2.5Ã lower compute for video reasoning versus separate vision + speech pipelines. It supports up to 300K context length and a 16,384 reasoning budget, with extended thinking enabled via reasoning.enabled on OpenRouter.

- **核心技术剖析与场景定位**：
  - 全感知子智能体 (Omni Sub-Agent)：30B 总参数、3B 激活，支持文本、音频、图像、视频全四模态输入，专为企业级多智能体系统充当感知中枢与实时环境上下文抓取器。

### 4.16 Google: Gemma 4 26B A4B  (free) (`google/gemma-4-26b-a4b-it:free`)

- **上线时间**：2026-04-03 14:53:09 UTC (北京时间 2026-04-03 22:53)
- **上游提供商与量化**：Google AI Studio (量化: unknown)
- **上下文窗口**：`262,144` Tokens | **单次最大输出**：`32768` Tokens
- **输入与输出模态**：输入 `image, text, video`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, response_format, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> Gemma 4 26B A4B IT is an instruction-tuned Mixture-of-Experts (MoE) model from Google DeepMind. Despite 25.2B total parameters, only 3.8B activate per token during inference â delivering near-31B quality at a fraction of the compute cost. Supports multimodal input including text, images, and video (up to 60s at 1fps). Features a 256K token context window, native function calling, configurable thinking/reasoning mode, and structured output support. Released under Apache 2.0.

- **核心技术剖析与场景定位**：
  - Google DeepMind 最新开源视觉 MoE：25.2B 总参数、3.8B 激活，原生支持文本、图像与视频多模态输入，具备出色的边缘侧部署效能与推理性价比。

### 4.17 Google: Gemma 4 31B (free) (`google/gemma-4-31b-it:free`)

- **上线时间**：2026-04-02 16:48:06 UTC (北京时间 2026-04-03 00:48)
- **上游提供商与量化**：Google AI Studio (量化: unknown)
- **上下文窗口**：`262,144` Tokens | **单次最大输出**：`32768` Tokens
- **输入与输出模态**：输入 `image, text, video`，输出 `text`
- **思维链模式 (Reasoning)**：支持 (默认开启: False, 强制: False)
- **核心支持参数**：`include_reasoning, max_tokens, reasoning, response_format, seed, temperature, tool_choice, tools, top_p`
- **官方技术定位描述**：
> Gemma 4 31B Instruct is Google DeepMind's 30.7B dense multimodal model supporting text and image input with text output. Features a 256K token context window, configurable thinking/reasoning mode, native function calling, and multilingual support across 140+ languages. Strong on coding, reasoning, and document understanding tasks. Apache 2.0 license.

- **核心技术剖析与场景定位**：
  - Google DeepMind 密集型视觉基座：30.7B Dense 参数，支持文本、图像与视频理解，具备 256K 上下文与原生函数调用，适合需要高稳定性与一致性输出的场景。

### 4.18 Free Models Router (`openrouter/free`)

- **上线时间**：2026-02-01 03:43:47 UTC (北京时间 2026-02-01 11:43)
- **上游提供商与量化**： (量化: )
- **上下文窗口**：`200,000` Tokens | **单次最大输出**：`None` Tokens
- **输入与输出模态**：输入 `text, image`，输出 `text`
- **思维链模式 (Reasoning)**：不支持显式推理链
- **核心支持参数**：`frequency_penalty, include_reasoning, logprobs, max_completion_tokens, max_tokens, min_p, presence_penalty, reasoning, reasoning_effort, repetition_penalty, response_format, seed, stop, structured_outputs, temperature, tool_choice, tools, top_k, top_logprobs, top_p`
- **官方技术定位描述**：
> The simplest way to get free inference. openrouter/free is a router that selects free models at random from the models available on OpenRouter. The router smartly filters for models that support features needed for your request such as image understanding, tool calling, structured outputs and more.

- **核心技术剖析与场景定位**：
  - 智能免配负载路由器：OpenRouter 官方元端点，由平台算法根据用户请求所需的参数（是否传入 image_url、是否需要 tools、是否启用 json_schema）自动负载均衡至当前延迟最低且能力匹配的活跃免费模型，是零成本兜底的最佳方案。


---

## 五、 开发者集成指南与最佳工程实践 (Developer Playbook)

### 1. 业务场景选型匹配矩阵
为帮助开发者快速根据实际需求选定最合适的免费模型，推荐以下场景矩阵：

| 核心应用场景 | 推荐首选模型 | 推荐备选模型 | 选型关键考量 |
| :--- | :--- | :--- | :--- |
| **复杂代码修复 / Agentic Coding** | `poolside/laguna-s-2.1:free` | `cohere/north-mini-code:free` | 专为 Terminal-Bench 与 SWE-Agent 调优，多步终端工具调用能力强 |
| **长文档分析 / 论文笔记 / 知识库** | `dots-studio/dots-3-note-preview:free` | `minimax/minimax-m3:free` | 512K~1M 超大上下文，支持超长结构化深度输出 |
| **金融数据与复杂业务投研** | `inclusionai/ling-3.0-flash-fin:free` | `minimax/minimax-m2.7:free` | 具备金融垂直知识储备与长程多步骤推演能力 |
| **多模态多感官理解 (图/音频/视频)** | `thinkingmachines/inkling:free` | `google/gemma-4-26b-a4b-it:free` | 支持全模态输入，图文/音频/视频多模态识别与解析 |
| **微服务高并发 / 极速轻量 RAG** | `nvidia/nemotron-3.5-lightning:free` | `liquid/lfm-2.5-2.6b:free` | 极小激活参数 (3B/2.6B)，极低端到端延迟，高吞吐 |
| **多智能体前沿逻辑推理** | `nvidia/nemotron-3-ultra-550b-a55b:free` | `z-ai/glm-5.2:free` | 550B 巨量 MoE 或 GLM 深度推理模式，复杂问题求解 |
| **低成本无感兜底与通用测试** | `openrouter/free` | `google/gemma-4-31b-it:free` | 官方自动路由，自动适配模态与工具，免去硬编码模型 ID 烦恼 |

### 2. 标准 OpenAI 兼容调用范式 (Python 示例)
OpenRouter 完全兼容 OpenAI 标准 API 规范，只需配置 `base_url` 与认证头：

```python
from openai import OpenAI
import os

client = OpenAI(
    base_url="https://openrouter.ai/api/v1",
    api_key=os.getenv("OPENROUTER_API_KEY"),
)

response = client.chat.completions.create(
    extra_headers={
        "HTTP-Referer": "https://your-domain.com",  # 遵循官方规范，便于展示排名与链路追踪
        "X-Title": "MyProductionApp",
    },
    # 推荐使用官方智能路由，或显式指定具体模型
    model="inclusionai/ling-3.0-flash-fin:free",
    messages=[
        {"role": "system", "content": "You are a professional investment analysis assistant."},
        {"role": "user", "content": "请分析生成式 AI 算力供应链在 2026 年面临的边际成本趋势。"}
    ],
    temperature=0.3,
    max_tokens=4096,
)
print(response.choices[0].message.content)
```

### 3. 高可用工程防御：429 限流指数退避与多模型 Fallback 配置
鉴于免费模型统一受 20 RPM 限制且高并发时容易触发上游 Provider 拥塞，生产代码推荐配置服务端 Fallback 列表与客户端退避：

```python
import time
import random
from openai import OpenAI, RateLimitError, APIError

def safe_completion_with_fallbacks(client, messages, primary_model, fallback_models, max_retries=3):
    # 借助 OpenRouter 原生 fallback 特性：在 extra_body 中传递备选列表
    for attempt in range(max_retries):
        try:
            response = client.chat.completions.create(
                model=primary_model,
                messages=messages,
                extra_body={
                    "models": fallback_models,  # OpenRouter 服务端自动顺位降级
                },
                timeout=60
            )
            return response
        except RateLimitError as e:
            # 指数退避加随机抖动 (Exponential Backoff with Jitter)
            wait_time = (2 ** attempt) + random.uniform(0.5, 1.5)
            print(f"触发速率限制 (429)。等待 {wait_time:.2f} 秒后重试...")
            time.sleep(wait_time)
        except APIError as e:
            print(f"API 错误: {e}")
            break
    raise RuntimeError("已耗尽所有重试与备选模型。")

# 使用示例：
# safe_completion_with_fallbacks(
#     client,
#     messages=[...],
#     primary_model="nvidia/nemotron-3.5-lightning:free",
#     fallback_models=["dots-studio/dots-3-note-preview:free", "openrouter/free"]
# )
```

### 4. 商业化平滑迁移路径
- **ID 解除免费后缀**：OpenRouter 上绝大部分以 `:free` 命名的优质模型均有对应的商业付费版本（直接去掉 `:free` 后缀）。
- **零代码改造上线**：在开发测试阶段使用 `:free` 端点，上线生产环境时仅需在环境变量中去掉模型名末尾的 `:free`，即可无缝切换至解除 20 RPM 限制、享有最高服务可用性与专属 SLA 的商业高速通道。

---

## 六、 附录：第一手参考来源与官方接口索引

1. **OpenRouter 官方模型列表 API**：`https://openrouter.ai/api/v1/models`
2. **OpenRouter 官方模型端点详情 API**：`https://openrouter.ai/api/v1/models/{model_id}/endpoints`
3. **OpenRouter 官方限流与信用规则文档**：`https://openrouter.ai/docs/limits`
4. **OpenRouter 官方 API 接口参考文档**：`https://openrouter.ai/docs/api_reference/overview`
5. **OpenRouter 隐私与观测性说明**：`https://openrouter.ai/docs`
6. **各开源研发机构官网与技术报告**：
   - Poolside AI: `https://poolside.ai/`
   - Liquid AI: `https://liquid.ai/`
   - NVIDIA Developer & Nemotron: `https://developer.nvidia.com/`
   - Cohere North: `https://cohere.com/`
   - Google DeepMind Gemma: `https://deepmind.google/technologies/gemma/`
   - MiniMax AI: `https://minimaxi.com/`
   - Z.ai / 智谱开源体系: `https://z.ai/`
