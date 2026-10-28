# Domestic pricing audit, 2026-10-28

Prices below use the site's existing nominal balance convention: CNY 1 paid =
1 balance unit, even where the UI labels the unit USD. Group multipliers remain
unchanged. Prices are per million tokens, stored by the API as per-token values.

## Corrected catalog

| Model | Input | Output | Cache read | Cache write 5m / 1h |
| --- | ---: | ---: | ---: | ---: |
| DeepSeek Flash / V4.1 Flash, off-peak | 1 | 4 | 0.02 | 0 / 0 |
| DeepSeek V4 Pro / 0813, off-peak | 4.5 | 13.5 | 0.15 | 0 / 0 |
| GLM 5.2 / 5.3 | 8 | 28 | 2 | 0 / 0 |
| GLM 5.3 Flash | 0.8 | 2.8 | 0.23 | 0 / 0 |
| GLM 5.3 FlashX | 2 | 7 | 0.57 | 0 / 0 |
| GLM 5.1, input <32K / >=32K | 6 / 8 | 24 / 28 | 1.3 / 2 | 0 / 0 |
| GLM 5 Turbo, input <32K / >=32K | 5 / 7 | 22 / 26 | 1.2 / 1.8 | 0 / 0 |
| Kimi K3 | 20 | 100 | 2 | 20 / 40 |
| Kimi K3.1 Pro | 13 | 68 | 1.3 | 13 / 26 |
| Kimi K2.7 Code | 6.5 | 27 | 1.3 | 0 / 0 |
| MiMo V2.6 Flash | 1 | 2 | 0.02 | 0 / 0 |
| HY3 | 1 | 4 | 0.25 | 0 / 0 |
| HY4 Preview | 6 | 18 | 0.3 | 0 / 0 |
| MiniMax M3, input <=512K / >512K | 2.1 / 4.2 | 8.4 / 16.8 | 0.42 / 0.84 | 0 / 0 |
| MiniMax M2.7 | 2.1 | 8.4 | 0.42 | 0 / 0 |
| Qwen 3.7 Max / 3.8 Max | 12 | 36 | Existing fallback retained | Unchanged |
| Qwen 3.8 Flash | 0.8 | 2.7 | Existing fallback retained | Unchanged |

DeepSeek peak periods are 09:00–12:00 and 14:00–18:00 Asia/Shanghai on
weekdays, multiplying the off-peak prices by 2. The old configuration already
used peak prices as its base, then doubled them again. Flash cache reads also
used an outdated price. Intervals are half-open in time; no second DeepSeek
multiplier applies to explicit channel prices.

The catalog supplies matching OpenAI and native DeepSeek/Zhipu/Kimi/MiniMax
cards. B's domestic channel additionally covers its previously unbound expensive
domestic group and native DeepSeek group. C retains all existing channel links.
No historical usage, balances, account routing, or group multipliers are changed.

## Limits requiring separate evidence or implementation

- The time-pricing schema supports weekdays/weekends, not legal-holiday dates.
  DeepSeek's published holiday exception therefore is not fully represented.
  There are no remaining 2026 Chinese public holidays after this audit date.
- Qwen 3.8 cache prices require the provider console. The operator explicitly
  deferred those prices; null overrides preserve existing fallback behavior.
  Qwen 3.7 implicit/explicit cache modes also require distinct rates, so no
  guessed common cache rate was introduced.
- MiniMax M2.7 Highspeed and the supplier's `mimo-v2.5` alias remain unmodified:
  the current official pages do not establish an applicable price for them.
  MiMo's own V2.5 endpoint was retired on October 21; a reseller alias may differ.
- Kimi 1-hour cache pricing is configured, but the OpenAI usage path currently
  records aggregate cache creation, without separate 5-minute/1-hour buckets.
  A 1-hour write therefore cannot yet be verified end to end on this path.
- GLM thresholds use 32,000 tokens; MiniMax uses 512,000 tokens. The existing
  interval engine uses `(min, max]`, so GLM's high tier starts above 31,999.
- Search/Fetch request fees are separate products, not text-token pricing.
  They were not added to chat/completion charges.

## Sources

- [DeepSeek](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/)
- [GLM](https://docs.bigmodel.cn/cn/guide/start/pricing)
- [Kimi](https://platform.kimi.com/docs/pricing/chat)
- [MiMo](https://mimo.mi.com/docs/zh-CN/price/pay-as-you-go)
- [MiniMax](https://platform.minimax.cn/docs/pricing/overview)
- [Tencent TokenHub: HY and MiniMax M2.7](https://cloud.tencent.com/document/product/1823/130055)
- [Qwen](https://help.aliyun.com/zh/model-studio/model-pricing)
- [Qwen cache exceptions](https://help.aliyun.com/zh/model-studio/context-cache)

## Verification and rollback

The exact proposed catalog is tested against both B and C billing services,
including peak boundaries, weekends, group multipliers, platform variants and
context-tier boundaries. A production API snapshot and optimistic before-state
comparison precede each update; API readback verifies every configured field.
Backups and after-state evidence are retained under
`/www/backups/sub2api-b/domestic-pricing-20261028/`.
