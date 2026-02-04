<script setup lang="ts">
import { computed } from "vue";
import type { RunUsageSummary } from "../runUsage.ts";

type SegmentKey = "input" | "cacheRead" | "cacheCreate" | "cached" | "output";

const props = defineProps<{
  usage: RunUsageSummary | null;
  status?: string;
}>();

type Segment = {
  key: SegmentKey;
  label: string;
  tokens: number;
  flex: number;
};

function formatTokens(v: number): string {
  const n = Number(v);
  if (!Number.isFinite(n) || n <= 0) return "0";
  if (n < 1000) return String(Math.round(n));
  if (n < 1_000_000) return `${trim1(n / 1000)}k`;
  if (n < 1_000_000_000) return `${trim1(n / 1_000_000)}M`;
  return `${trim1(n / 1_000_000_000)}B`;
}

function trim1(v: number): string {
  const s = v.toFixed(1);
  return s.endsWith(".0") ? s.slice(0, -2) : s;
}

function formatUSD(v: number): string {
  const n = Number(v);
  if (!Number.isFinite(n) || n <= 0) return "";
  if (n < 0.01) return `$${n.toFixed(4)}`;
  if (n < 1) return `$${trimTrailingZeros(n.toFixed(3))}`;
  return `$${n.toFixed(2)}`;
}

function trimTrailingZeros(raw: string): string {
  const s = String(raw);
  if (!s.includes(".")) return s;
  return s.replace(/0+$/, "").replace(/\.$/, "");
}

const placeholderText = computed(() => {
  const s = String(props.status ?? "").trim();
  if (s === "running" || s === "queued") return "Tokens: counting…";
  return "";
});

const costText = computed(() => {
  const u = props.usage;
  if (!u?.totalCostUSD) return "";
  return formatUSD(u.totalCostUSD);
});

const segments = computed<Segment[]>(() => {
  const u = props.usage;
  if (!u) return [];

  const parts: Array<{ key: SegmentKey; label: string; tokens: number }> = [
    { key: "input", label: "Input", tokens: u.inputTokens },
    { key: "cacheRead", label: "Cache read", tokens: u.cacheReadInputTokens },
    { key: "cacheCreate", label: "Cache create", tokens: u.cacheCreationInputTokens },
    { key: "cached", label: "Cached input", tokens: u.cachedInputTokens },
    { key: "output", label: "Output", tokens: u.outputTokens },
  ].filter((p) => p.tokens > 0);

  const total = parts.reduce((acc, p) => acc + p.tokens, 0);
  if (!total) return [];

  const scale = 1000;
  return parts.map((p) => ({
    ...p,
    flex: Math.max(1, Math.round((p.tokens / total) * scale)),
  }));
});

const summaryTitle = computed(() => {
  const u = props.usage;
  if (!u) return "";

  const parts: string[] = [];
  parts.push(`Input ${formatTokens(u.inputTokens)}`);
  if (u.cacheReadInputTokens) parts.push(`Cache read ${formatTokens(u.cacheReadInputTokens)}`);
  if (u.cacheCreationInputTokens) parts.push(`Cache create ${formatTokens(u.cacheCreationInputTokens)}`);
  if (u.cachedInputTokens) parts.push(`Cached input ${formatTokens(u.cachedInputTokens)}`);
  parts.push(`Output ${formatTokens(u.outputTokens)}`);
  parts.push(`Total ${formatTokens(u.totalTokens)}`);
  if (costText.value) parts.push(`Cost ${costText.value}`);
  return parts.join(" · ");
});
</script>

<template>
  <details v-if="usage" class="usageMeter">
    <summary class="usageSummary" :title="summaryTitle" aria-label="Token usage">
      <div class="usageBar" aria-hidden="true">
        <div
          v-for="seg in segments"
          :key="seg.key"
          class="usageSeg"
          :class="seg.key"
          :style="{ flexGrow: seg.flex }"
          :title="`${seg.label}: ${formatTokens(seg.tokens)}`"
        ></div>
      </div>
      <div class="usageNumbers">
        <span class="mono">{{ formatTokens(usage.inputTotalTokens) }}</span>
        <span class="usageArrow" aria-hidden="true">→</span>
        <span class="mono">{{ formatTokens(usage.outputTokens) }}</span>
        <span v-if="costText" class="mono usageCost">{{ costText }}</span>
      </div>
    </summary>

    <div class="usageDetails">
      <div class="usageGrid">
        <div class="usageRow">
          <span class="k">Input</span>
          <span class="mono">{{ formatTokens(usage.inputTokens) }}</span>
        </div>
        <div v-if="usage.cacheReadInputTokens" class="usageRow">
          <span class="k">Cache read</span>
          <span class="mono">{{ formatTokens(usage.cacheReadInputTokens) }}</span>
        </div>
        <div v-if="usage.cacheCreationInputTokens" class="usageRow">
          <span class="k">Cache create</span>
          <span class="mono">{{ formatTokens(usage.cacheCreationInputTokens) }}</span>
        </div>
        <div v-if="usage.cachedInputTokens" class="usageRow">
          <span class="k">Cached input</span>
          <span class="mono">{{ formatTokens(usage.cachedInputTokens) }}</span>
        </div>
        <div class="usageRow">
          <span class="k">Output</span>
          <span class="mono">{{ formatTokens(usage.outputTokens) }}</span>
        </div>
        <div class="usageRow">
          <span class="k">Total</span>
          <span class="mono">{{ formatTokens(usage.totalTokens) }}</span>
        </div>
        <div v-if="costText" class="usageRow">
          <span class="k">Cost</span>
          <span class="mono">{{ costText }}</span>
        </div>
        <div class="usageRow">
          <span class="k">Source</span>
          <span class="mono">{{ usage.source }}</span>
        </div>
      </div>

      <div v-if="usage.models?.length" class="usageModels">
        <div class="usageModelsTitle">Models</div>
        <div v-for="m in usage.models" :key="m.model" class="usageModelRow">
          <span class="mono usageModelName" :title="m.model">{{ m.model }}</span>
          <span class="mono">{{ formatTokens(m.totalTokens) }}</span>
          <span v-if="m.costUSD" class="mono usageModelCost">{{ formatUSD(m.costUSD) }}</span>
        </div>
      </div>
    </div>
  </details>

  <div v-else-if="placeholderText" class="usageMeter placeholder" aria-label="Token usage">
    {{ placeholderText }}
  </div>
</template>

