# AI / SaaS Prepaid Credits 的 Chargeback 公开做法

> 调研快照：2026-07-24。本文记录公开可验证的产品政策与支付平台流程，不是 IdeaBoard 当前实现或法律意见。除 Stripe 外，供应商 Terms 通常不会公开其内部的余额追回或裁决后恢复规则；不能把未披露流程当作行业事实。

## 可执行结论

公开材料可验证的模式是：**预付余额通常被定义为受服务条款约束、不可提现的使用权，服务商普遍保留暂停或终止服务的权利。** 对“chargeback 发起即暂停”这一更具体做法，本次有 Replicate 的直接公开先例，不能外推为所有厂商的明示政策。

- **正式 dispute 创建时**：立即阻止计算消费是有公开依据的保守做法。Replicate 的 Terms 明定，发起 chargeback 后可立即暂停服务访问；Stripe 也会在 dispute 存续期间先从商户余额扣走争议金额和 fee。
- **充值 / 购买**：公开条款通常写的是“暂停 access/service”，而不是单独公开“关闭 top-up”。因此冻结消费并同时禁用新购买，是合理的风险控制推断，不应声称为普遍的公开行业规则。对已经成功支付但 webhook 尚未到达的订单，仍应按支付事实入账、但保持账户不可消费，避免账实不一致。
- **扣回 Idea**：没有找到 OpenAI、Anthropic、Midjourney、Runway、Replicate、ElevenLabs、Canva 或 Adobe 的公开政策，明确承诺或描述“按原订单自动扣回剩余 credits”。这应是 IdeaBoard 的内部 ledger 策略：从可用余额扣回至多原订单发放量，余额不足只记录未追回额，不制造用户负余额。
- **胜诉恢复**：Stripe 在商户侧会返还争议款；但上述产品没有公开“胜诉后自动恢复用户 credits / 自动解冻”的承诺。建议作为内部状态机：`dispute won` 恢复本次扣回并解除支付冻结；`lost` 保留扣回和冻结，进入人工审核。需要覆盖罕见的 `late win`。

这比“冻结 120 天后才发放 credits”更符合公开支付流程：卡支付 dispute 的常见窗口是约 120 天，但 Stripe 所述的资金扣留发生在 dispute 被正式发起时，不是支付成功时。

## 公开事实

| 主题 | 可验证事实 | 对 IdeaBoard 的含义 |
| --- | --- | --- |
| Stripe 资金流 | 正式 dispute 期间，card network 从 Stripe 扣款，Stripe 再从商户余额扣除争议金额和 dispute fee；资金一直被持有至争议结束。最终由发卡行裁决为 `won` 或 `lost`；罕见情况下可从 `lost` 变为 `late win`，Stripe 将资金返还商户余额。 | `dispute.created` 不能被视为普通“支付失败”；它是已交付 credits 后的事后资金风险。处理需可逆并能响应最终裁决和 late win。 |
| Replicate（预付 credits） | Terms 定义 `Prepaid Balance`：用户预付后按实际使用扣减；credits 不构成财产权、不可兑换/退款/转让，所有 prepaid credit 销售为 final。余额用尽且未补足、或 overage 未支付时，可暂停服务直至余额恢复/付款。更直接地，Terms 8.4 写明：发起 chargeback 后，Replicate 保留“immediately suspend”服务访问的权利。 | 这是本次样本中最直接的 AI/SaaS 公共先例：chargeback 发起即暂停 access。它没有公开要求“扣回具体 credits”，也未公开胜诉自动恢复。 |
| Runway（订阅 / credits） | Terms 允许因未能及时从 Payment Provider 收款而立即、无通知地 suspend 或 terminate 服务；退款一经发起即终止 subscription。Terms 也明确未使用期间一般不退款或给 credits。 | 支付异常时中止服务和不把未用权益当现金返还，是公开常见条款；未找到 chargeback 专项或 credits 追回细则。 |
| OpenAI、Anthropic | OpenAI Terms 允许在认为用户违反 Terms 时 suspend / terminate access，并对因违规终止的 subscription 不退款。Anthropic Consumer Terms 允许在认为违反 Terms 或法律要求时 suspend / terminate access，违规终止通常无退款。 | 大型 AI 服务保留广泛的服务暂停权，但公开 Terms 未提供 chargeback 到余额扣回、购买禁用、胜诉恢复的细粒度规则。 |
| ElevenLabs | Terms 明确：账户 closed 或 terminated 时，相关服务的所有 unused credits（含 character credits）将 forfeiture；并保留临时或永久 suspend / terminate access 的权利。 | 这是明确的 credits forfeiture 条款，但触发条件是账户关闭/终止，**不是** chargeback 本身；不能据此推出争议创建即自动没收余额或胜诉恢复。 |
| Replicate、Runway 等一般 SaaS 条款 | 服务商通常保留因违约、未付款、疑似不实信息或法律原因暂停/终止访问的权利。 | 只能支持“账户级限制”这一共同模式，不能推出统一的自动化 credits 回收算法。 |

## 建议的最小产品策略

以下是基于上述事实的设计建议，而非“其他厂商公开承诺都这样做”。

1. 收到经签名校验且幂等的 `charge.dispute.created`：将账户设为 `payment_dispute_frozen`，立刻禁止 Task/AI 消费；使未完成 Checkout Session 失效，并在 UI/API 拒绝新的 top-up。保留查看账单和联系支持的能力。
2. 用关联订单的 grant 作为上限，从 **available** Idea 扣回；扣回不超过当前余额，也不超过该订单发放的 Idea。余额不足则记录 `unrecovered_credits`，不创建可继续消费的负余额。
3. 在冻结期间，已由支付提供方确认成功的订单仍按幂等 webhook 正常记入 ledger，但不能被消费；不要将“账户冻结”变成丢失已收付款的账务事件。
4. `dispute won` 或 `late win`：冲销本次 dispute 的追回 posting，解除对应支付冻结（若无其他冻结原因）。`dispute lost`：保留追回和冻结，交由人工按证据、账户历史和法定义务决定是否永久关闭或有限解封。
5. 支付方式不要按名称一刀切。应由 payment provider 返回的 dispute/退款事件驱动；不同 Local Payment Method 的争议窗口和处理者不同，不能假设“微信支付没有任何争议路径”。

## 关键边界

- **不应自动追溯已消耗的 Idea**：已消耗部分对应已经发生的模型/API 成本。将未追回部分记为审计字段，优于把用户钱包余额改成负数并引入未设计的债务与催收语义。
- **不要把 dispute 开立等同于败诉**：资金虽然已从商户 balance 预扣，但最终仍可能胜诉。因此扣回与冻结需要可逆、可审计、严格按 dispute ID 幂等。
- **“禁止充值”是策略选择**：它能避免账户在争议期通过新资金恢复使用，但公开来源只普遍支持暂停服务，并未证明每家产品都禁用支付入口。若业务希望允许补款，应要求人工审核或将新款项隔离，不能自动解冻。

## 来源

全部链接访问于 2026-07-24。

- [Stripe: How disputes work](https://docs.stripe.com/disputes/how-disputes-work)：正式 dispute 期间扣商户余额、结果、late win，以及卡支付常见约 120 天窗口。
- [Stripe: Respond to disputes](https://docs.stripe.com/disputes/responding)：`won` 时争议金额返还、`lost` 时退款永久生效。
- [Replicate Terms of Service](https://replicate.com/terms)：§4.2 Prepaid Credit、§4.8 Chargebacks and Dispute Resolution、§9 Suspension & Termination。
- [Runway Terms of Use](https://runwayml.com/terms-of-use/)：§8 Fees and Purchase Terms、§14 Termination。
- [OpenAI Terms of Use](https://openai.com/policies/terms-of-use/)：Termination / suspension 与退款条款。
- [Anthropic Consumer Terms](https://www.anthropic.com/legal/consumer-terms)：Termination / suspension 与退款条款。
- [ElevenLabs Terms of Service](https://elevenlabs.io/terms-of-service)：账户关闭/终止时 unused credits forfeiture。

## 未验证或未发现的公开规则

本次快速一手资料检索未发现以下厂商公开披露其 chargeback 触发的 credits ledger 细则：Midjourney、ElevenLabs、Canva、Adobe。它们的公开 Terms / Help 多为一般付款、退款、账户暂停或合同争议条款，不能作为“发生银行卡 chargeback 后自动扣回 credits / 禁止充值 / 胜诉自动恢复”的证据。

同样，以上已列供应商也没有在本次检索范围内公开一条可引用的“按原购买批次追回全部未消耗 credits，并在 Stripe dispute 胜诉后自动返还”的规则。因此 IdeaBoard 应将该流程明确写成自身政策和可审计的 ledger 机制。
