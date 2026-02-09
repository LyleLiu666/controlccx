<script setup lang="ts">
import { onMounted } from "vue";
import { useAudit } from "../composables/useAudit";

const emit = defineEmits<{
  (e: "back"): void;
}>();

const {
  loading,
  loadingMore,
  loadingDetail,
  loadingMeta,
  error,
  sources,
  retention,
  entries,
  pageNumber,
  hasPrevPage,
  hasNextPage,
  selectedID,
  detail,
  querySources,
  queryKeyword,
  queryFrom,
  queryTo,
  queryTaskID,
  queryRunID,
  queryStreams,
  init,
  search,
  loadPrevPage,
  loadNextPage,
  selectEntry,
  resetFilters,
} = useAudit();

const streamOptions = ["stdout", "stderr", "system", "assistant"] as const;

function fmtTime(iso: string): string {
  const d = new Date(String(iso ?? "").trim());
  if (Number.isNaN(d.getTime())) return String(iso ?? "");
  return d.toLocaleString();
}

function onSearch() {
  void search();
}

function onReset() {
  resetFilters();
  void search();
}

function onSelect(id: string) {
  void selectEntry(id);
}

function onPrevPage() {
  void loadPrevPage();
}

function onNextPage() {
  void loadNextPage();
}

onMounted(() => {
  void init();
});
</script>

<template>
  <section class="panel auditPanel">
    <div class="auditHeader">
      <div class="auditHeaderLeft">
        <div class="auditTitle">审计日志</div>
        <div class="auditSubtitle">
          统一检索秘书与任务关键审计日志索引（只读）
        </div>
      </div>
      <span class="h2Spacer"></span>
      <button
        type="button"
        class="contextCloseBtn"
        @click="emit('back')"
        aria-label="Close"
      >
        <span aria-hidden="true">×</span>
      </button>
    </div>

    <div class="auditMetaRow">
      <span v-if="retention" class="tinyHint">
        留存：{{ retention.days }} 天 / 每来源
        {{ retention.max_rows_per_source }} 条
      </span>
      <span v-if="retention?.last_run?.run_at" class="tinyHint mono">
        最近 GC：{{ fmtTime(retention.last_run.run_at) }}（{{
          retention.last_run.duration_ms
        }}
        ms）
      </span>
      <span v-if="loadingMeta" class="tinyHint">元数据加载中…</span>
    </div>

    <div class="auditFilters">
      <label class="full">
        <span>关键词</span>
        <input
          v-model="queryKeyword"
          placeholder="如：blocked / safety.autopilot / run_id"
        />
      </label>
      <label>
        <span>task_id</span>
        <input v-model="queryTaskID" placeholder="task-xxx" />
      </label>
      <label>
        <span>run_id</span>
        <input v-model="queryRunID" placeholder="run-xxx" />
      </label>
      <label>
        <span>from</span>
        <input v-model="queryFrom" type="datetime-local" />
      </label>
      <label>
        <span>to</span>
        <input v-model="queryTo" type="datetime-local" />
      </label>
      <div class="auditFilterGroup full">
        <span>来源</span>
        <label v-for="item in sources" :key="item.source" class="auditCheck">
          <input v-model="querySources" :value="item.source" type="checkbox" />
          <span>{{ item.label }}</span>
        </label>
      </div>
      <div class="auditFilterGroup full">
        <span>task_log streams</span>
        <label v-for="stream in streamOptions" :key="stream" class="auditCheck">
          <input v-model="queryStreams" :value="stream" type="checkbox" />
          <span class="mono">{{ stream }}</span>
        </label>
      </div>
      <div class="auditActions full">
        <button
          type="button"
          class="primary"
          @click="onSearch"
          :disabled="loading"
        >
          查询
        </button>
        <button type="button" @click="onReset" :disabled="loading">重置</button>
      </div>
      <div class="auditPager full">
        <button
          type="button"
          @click="onPrevPage"
          :disabled="loadingMore || !hasPrevPage"
        >
          上一页
        </button>
        <span class="tinyHint">第 {{ pageNumber }} 页</span>
        <button
          type="button"
          @click="onNextPage"
          :disabled="loadingMore || !hasNextPage"
        >
          下一页
        </button>
        <span v-if="loadingMore" class="tinyHint">翻页中…</span>
      </div>
    </div>

    <div v-if="error" class="modalError">{{ error }}</div>

    <div class="auditBody">
      <div class="auditList">
        <div v-if="loading" class="loading">加载中…</div>
        <div v-else-if="entries.length === 0" class="empty">暂无数据</div>
        <button
          v-for="item in entries"
          :key="item.id"
          type="button"
          class="auditRow"
          :class="{ active: item.id === selectedID }"
          @click="onSelect(item.id)"
        >
          <div class="auditRowTop">
            <span class="mono">{{ fmtTime(item.time) }}</span>
            <span class="pill low mono">{{ item.source }}</span>
          </div>
          <div class="auditRowTitle">{{ item.title }}</div>
          <div class="auditRowSummary">{{ item.summary }}</div>
          <div class="auditRowRaw mono">{{ item.raw_preview }}</div>
        </button>
      </div>

      <div class="auditDetail">
        <div v-if="loadingDetail" class="loading">详情加载中…</div>
        <template v-else-if="detail">
          <div class="auditDetailHead">
            <div class="auditDetailTitle">{{ detail.title }}</div>
            <div class="tinyHint mono">{{ detail.id }}</div>
          </div>
          <div
            v-if="detail.meta?.kv_cache || detail.meta?.provider_receipt"
            class="auditDetailInsights"
          >
            <div v-if="detail.meta?.kv_cache" class="auditInsight">
              <div class="auditInsightK">KV Cache</div>
              <div class="auditInsightV mono">
                read={{
                  detail.meta?.kv_cache?.cache_read_input_tokens ??
                  detail.meta?.kv_cache?.prompt_cached_tokens ??
                  "-"
                }}
                · create={{
                  detail.meta?.kv_cache?.cache_creation_input_tokens ?? "-"
                }}
                · cached={{
                  detail.meta?.kv_cache?.cached_input_tokens ?? "-"
                }}
                · epoch={{ detail.meta?.kv_cache?.request_cache_epoch ?? "-" }}
              </div>
            </div>
            <div v-if="detail.meta?.provider_receipt" class="auditInsight">
              <div class="auditInsightK">Provider Receipt</div>
              <div class="auditInsightV mono">
                provider={{ detail.meta?.provider_receipt?.provider ?? "-" }} ·
                model={{
                  detail.meta?.provider_receipt?.model ??
                  detail.meta?.provider_receipt?.request_model ??
                  "-"
                }}
                · request_id={{
                  detail.meta?.provider_receipt?.request_id ?? "-"
                }}
                · status={{ detail.meta?.provider_receipt?.status_code ?? "-" }}
              </div>
            </div>
          </div>
          <pre class="auditDetailRaw">{{ detail.raw }}</pre>
          <pre v-if="detail.meta" class="auditDetailMeta">{{
            JSON.stringify(detail.meta, null, 2)
          }}</pre>
        </template>
        <div v-else class="empty">请选择一条审计记录查看详情</div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.auditPanel {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  box-sizing: border-box;
}

.auditHeader {
  display: flex;
  align-items: center;
  gap: 12px;
}

.auditHeaderLeft {
  min-width: 0;
}

.auditTitle {
  font-size: 20px;
  font-weight: 700;
}

.auditSubtitle {
  color: var(--muted);
  font-size: 13px;
}

.auditMetaRow {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.auditFilters {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
}

.auditFilters label {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.auditFilters label > span {
  color: var(--muted);
  font-size: 12px;
}

.auditFilters .full {
  grid-column: 1 / -1;
}

.auditFilterGroup {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}

.auditCheck {
  display: inline-flex !important;
  flex-direction: row !important;
  align-items: center;
  gap: 6px;
  color: var(--fg);
}

.auditActions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.auditPager {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.auditBody {
  display: grid;
  min-height: 0;
  flex: 1;
  grid-template-columns: minmax(360px, 0.9fr) minmax(0, 1.1fr);
  gap: 12px;
}

.auditList,
.auditDetail {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--line);
  border-radius: 12px;
  padding: 10px;
  overflow: auto;
}

.auditRow {
  display: flex;
  flex-direction: column;
  gap: 4px;
  text-align: left;
}

.auditRow.active {
  border-color: var(--accent);
}

.auditRowTop {
  display: flex;
  align-items: center;
  gap: 8px;
}

.auditRowTitle {
  font-weight: 600;
}

.auditRowSummary {
  color: var(--muted);
}

.auditRowRaw {
  color: var(--fg2);
  white-space: pre-wrap;
  word-break: break-word;
}

.auditDetailHead {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.auditDetailTitle {
  font-weight: 700;
}

.auditDetailInsights {
  display: grid;
  gap: 8px;
}

.auditInsight {
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 8px;
  background: color-mix(in oklab, var(--surface2) 84%, transparent);
  display: grid;
  gap: 6px;
}

.auditInsightK {
  font-size: 12px;
  color: var(--muted);
  font-weight: 700;
}

.auditInsightV {
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}

.auditDetailRaw,
.auditDetailMeta {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--surface2);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 8px;
}

.auditDetailMeta {
  color: var(--muted);
}

@media (max-width: 1100px) {
  .auditPanel {
    padding: 12px;
  }

  .auditFilters {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .auditBody {
    grid-template-columns: 1fr;
  }
}
</style>
