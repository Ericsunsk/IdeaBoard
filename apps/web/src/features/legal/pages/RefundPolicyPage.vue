<template>
  <PublicLegalPage title="退款政策" :version="REFUND_POLICY_VERSION">
    <p>本政策适用于通过 IdeaBoard Stripe Checkout 购买的 Idea。</p>

    <h2>1. 申请期限</h2>
    <p>
      退款资格从 Stripe 确认支付的时间起计算 14 天。目前不提供自助退款，请在期限内联系管理员。
    </p>

    <h2>2. 可退数量</h2>
    <p>
      可退 Idea 不超过该订单尚未退款的数量和当前账户可用余额中的较小值。已经消费的 Idea
      无法通过该笔订单退款。
    </p>

    <h2>3. 退款金额</h2>
    <p>
      商品退款金额按原订单 Idea 比例，以原支付币种的最小单位计算。税额退款以 Stripe Tax
      的最终处理结果为准。
    </p>

    <h2>4. 处理过程</h2>
    <p>
      管理员审核申请后，会通过对应的 Stripe 原支付交易处理退款。退款是否完成以及资金到账时间以
      Stripe 和支付机构的最终结果为准。
    </p>

    <h2>5. 支付争议</h2>
    <p>
      如发现未经授权的支付，请尽快联系我们。直接向发卡机构提出 Dispute
      可能触发账户支付与消费限制，直至争议收敛。
    </p>

    <h2>6. 联系我们</h2>
    <p>申请时请提供账户信息和充值记录中的交易时间；无需发送银行卡号、验证码或其他敏感支付信息。</p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'

const REFUND_POLICY_VERSION = '2026-08-01'
</script>
