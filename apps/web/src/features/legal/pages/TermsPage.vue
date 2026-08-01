<template>
  <PublicLegalPage title="服务条款" :version="TERMS_VERSION">
    <p>使用 IdeaBoard 即表示你同意本条款。购买或使用 Idea 前，请完整阅读以下内容。</p>

    <h2>1. 服务与账户</h2>
    <p>
      你应妥善保管账户，并对账户内发生的操作负责。不得利用服务侵害他人权益、规避安全措施或从事违法活动。
    </p>

    <h2>2. Idea 与购买</h2>
    <p>
      Idea 是仅用于 IdeaBoard 内功能消费的虚拟额度，不是货币、存款或可转让资产。购买价格以 Checkout
      展示为准，税费和本地币种换算由 Stripe 处理。
    </p>

    <h2>3. 支付与到账</h2>
    <p>
      支付由 Stripe 处理。浏览器返回 IdeaBoard 不代表支付成功；Idea 仅在 Stripe
      支付事实经系统确认后到账。
    </p>

    <h2>4. AI 内容</h2>
    <p>
      AI
      输出可能不准确或不适合特定用途。你应在使用、发布或依赖输出前自行审查，并确保输入和使用方式符合法律及第三方权利要求。
    </p>

    <h2>5. 服务限制</h2>
    <p>
      为维护安全、处理支付争议或遵守法律要求，我们可能限制相关账户操作。计划维护或不可控故障可能造成短暂中断。
    </p>

    <h2>6. 条款更新</h2>
    <p>条款更新后会发布新版本。每笔购买会保存你同意的条款版本，更新不会改写历史订单事实。</p>

    <h2>7. 联系我们</h2>
    <p>如对账户、购买或本条款有疑问，请通过产品内提供的客服渠道联系管理员。</p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'

const TERMS_VERSION = '2026-08-01'
</script>
