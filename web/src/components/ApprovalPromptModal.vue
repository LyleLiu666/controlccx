<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { ApprovalRequest, Task } from "../types";
import { decideTaskApproval, enterUnsafeTask, fetchTaskApprovals, isInstanceTokenRequiredError } from "../api";

const props = defineProps<{
  open: boolean;
  task: Task | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "tokenRequired", message: string): void;
  (e: "enterUnsafe", task: Task): void;
}>();

const approvals = ref<ApprovalRequest[]>([]);
const loading = ref(false);
const busy = ref(false);
const error = ref("");
const denyReason = ref("");
const rawOpen = ref(false);

const taskId = computed(() => String(props.task?.id ?? "").trim());

const pending = computed(() => approvals.value.filter((a) => a.status === "pending"));
const active = computed<ApprovalRequest | null>(() => pending.value[0] ?? null);

function riskLabel(risk: string): string {
  const v = String(risk ?? "").trim();
  if (v === "high") return "high risk";
  if (v === "medium") return "medium";
  if (v === "low") return "low";
  return v || "unknown";
}

function formatActionType(v: string): string {
  const s = String(v ?? "").trim();
  if (!s) return "Unknown";
  return s;
}

async function loadApprovals() {
  const id = taskId.value;
  if (!id) return;
  if (loading.value) return;
  loading.value = true;
  error.value = "";
  try {
    approvals.value = await fetchTaskApprovals(id, "pending");
  } catch (e: any) {
    if (isInstanceTokenRequiredError(e)) emit("tokenRequired", e?.message ?? "missing instance token");
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function decide(decision: "approve" | "deny") {
  const id = taskId.value;
  const ar = active.value;
  if (!id || !ar) return;
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    await decideTaskApproval({
      taskId: id,
      approvalId: ar.id,
      decision,
      reason: decision === "deny" ? denyReason.value : "",
    });
    denyReason.value = "";
    rawOpen.value = false;
    await loadApprovals();
    if (pending.value.length === 0) emit("close");
  } catch (e: any) {
    if (isInstanceTokenRequiredError(e)) emit("tokenRequired", e?.message ?? "missing instance token");
    error.value = e?.message ?? String(e);
  } finally {
    busy.value = false;
  }
}

async function enterUnsafe() {
  const id = taskId.value;
  if (!id) return;
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    const t = await enterUnsafeTask(id, "continue");
    emit("enterUnsafe", t);
  } catch (e: any) {
    if (isInstanceTokenRequiredError(e)) emit("tokenRequired", e?.message ?? "missing instance token");
    error.value = e?.message ?? String(e);
  } finally {
    busy.value = false;
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    approvals.value = [];
    denyReason.value = "";
    rawOpen.value = false;
    void loadApprovals();
  },
);

watch(
  () => taskId.value,
  (id, prev) => {
    if (!props.open) return;
    if (!id || id === prev) return;
    approvals.value = [];
    denyReason.value = "";
    rawOpen.value = false;
    void loadApprovals();
  },
);
</script>

<template>
  <div v-if="props.open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal smallModal approvalModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">需要审批</div>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody">
        <div v-if="error" class="modalError">{{ error }}</div>

        <div v-if="!props.task" class="tinyHint warn">未选择 run。</div>
        <template v-else>
          <div class="tinyHint">
            Run <span class="mono">{{ props.task.id.slice(0, 8) }}</span> ·
            <span class="mono">{{ props.task.worker_type }}</span>
            <span v-if="props.task.workdir" class="mono">· {{ props.task.workdir }}</span>
          </div>

          <div v-if="loading" class="confirmText">加载中…</div>
          <template v-else-if="!active">
            <div class="confirmText">没有待审批的请求。</div>
            <div class="tinyHint">如果你刚触发了一次工具调用，稍等几秒或关闭后重新打开。</div>
          </template>
          <template v-else>
            <div class="approvalCard">
              <div class="approvalTop">
                <div class="approvalAction mono">{{ formatActionType(active.action_type) }}</div>
                <span class="pill mono approvalRisk" :class="active.risk_level">{{ riskLabel(active.risk_level) }}</span>
              </div>
              <div v-if="active.summary" class="approvalSummary">{{ active.summary }}</div>
              <details
                class="approvalRaw"
                :open="rawOpen"
                @toggle="rawOpen = ($event.target as HTMLDetailsElement).open"
              >
                <summary class="mono">raw</summary>
                <pre class="mono">{{ JSON.stringify(active.raw ?? {}, null, 2) }}</pre>
              </details>
            </div>

            <label class="full">
              <div class="tinyHint">拒绝理由（可选）</div>
              <input v-model="denyReason" :disabled="busy" placeholder="(optional)" />
            </label>
          </template>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" class="warnBtn" :disabled="busy || !active" @click="decide('deny')">
          {{ busy ? "处理中…" : "Deny" }}
        </button>
        <button type="button" class="primary" :disabled="busy || !active" @click="decide('approve')">
          {{ busy ? "处理中…" : "Approve and continue" }}
        </button>
        <button type="button" class="dangerBtn" :disabled="busy || !props.task" @click="enterUnsafe">
          {{ busy ? "处理中…" : "Enter UNSAFE" }}
        </button>
      </div>
    </div>
  </div>
</template>
