<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { PerSkillVersionsListResponse } from "../types";
import {
  createSkillVersionBySkill,
  deleteSkillVersionBySkill,
  fetchSkillVersionsBySkill,
  restoreSkillVersionBySkill,
  updateSkillVersionBySkill,
} from "../api";

const props = defineProps<{
  open: boolean;
  skill: string;
  hasSource: boolean;
}>();

const emit = defineEmits<{
  (e: "close"): void;
}>();

const loading = ref(false);
const error = ref("");
const data = ref<PerSkillVersionsListResponse | null>(null);

const newId = ref("");
const newNote = ref("");
const creating = ref(false);
const deleting = ref<Map<string, boolean>>(new Map());
const restoring = ref<Map<string, boolean>>(new Map());
const updatingFromSource = ref(false);
const notice = ref("");

const title = computed(() => {
  const name = String(props.skill ?? "").trim();
  if (!name) return "版本快照";
  return `版本快照：${name}`;
});
const versions = computed(() => data.value?.versions ?? []);
const canCreate = computed(() => !!String(props.skill ?? "").trim());
const manifest = computed(() => data.value?.manifest ?? null);
const sourceType = computed(() => String(manifest.value?.source_type ?? "").trim());
const sourceRef = computed(() => String(manifest.value?.source_ref ?? "").trim());
const sourceBranch = computed(() => String(manifest.value?.source_branch ?? "").trim());
const sourceSubpath = computed(() => String(manifest.value?.source_subpath ?? "").trim());
const sourceRevision = computed(() => String(manifest.value?.source_revision ?? "").trim());
const sourceRefLabel = computed(() => {
  const t = sourceType.value;
  if (t === "git") return "Repo";
  if (t === "local") return "路径";
  if (t === "import") return "来源";
  return "source_ref";
});
const canUpdateFromSource = computed(() => {
  const name = String(props.skill ?? "").trim();
  return !!name && sourceType.value === "git";
});
const shortRevision = computed(() => {
  const r = sourceRevision.value;
  if (!r) return "";
  return r.length > 12 ? `${r.slice(0, 8)}…` : r;
});

async function refresh() {
  const name = String(props.skill ?? "").trim();
  if (!name) return;
  loading.value = true;
  error.value = "";
  try {
    data.value = await fetchSkillVersionsBySkill(name);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    void refresh();
  },
);
watch(
  () => props.skill,
  () => {
    if (!props.open) return;
    void refresh();
  },
);

async function createFromForm() {
  const name = String(props.skill ?? "").trim();
  if (!name) return;
  if (!canCreate.value) return;
  if (creating.value) return;
  creating.value = true;
  error.value = "";
  notice.value = "";
  try {
    await createSkillVersionBySkill(name, {
      id: newId.value.trim(),
      note: newNote.value.trim(),
    });
    newId.value = "";
    newNote.value = "";
    await refresh();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    creating.value = false;
  }
}

async function deleteByID(id: string) {
  const name = String(props.skill ?? "").trim();
  const v = String(id ?? "").trim();
  if (!name || !v) return;
  if (!window.confirm(`确认删除版本「${v}」？此操作不可撤销。`)) return;
  deleting.value.set(v, true);
  deleting.value = new Map(deleting.value);
  error.value = "";
  notice.value = "";
  try {
    await deleteSkillVersionBySkill(name, { id: v });
    await refresh();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    deleting.value.delete(v);
    deleting.value = new Map(deleting.value);
  }
}

async function restoreByID(id: string) {
  const name = String(props.skill ?? "").trim();
  const v = String(id ?? "").trim();
  if (!name || !v) return;
  if (!window.confirm(
    `确认恢复到该版本「${v}」？\n\n` +
      `这会覆盖 ~/.agent/skills/${name}，并影响所有已挂载该技能的 target。\n` +
      `系统会在恢复前自动生成一份快照用于回滚。`,
  )) {
    return;
  }

  restoring.value.set(v, true);
  restoring.value = new Map(restoring.value);
  error.value = "";
  notice.value = "";
  try {
    const out = await restoreSkillVersionBySkill(name, { id: v });
    if (out?.backup_id) {
      notice.value = `已恢复。已自动备份为 ${out.backup_id}。`;
    } else {
      notice.value = "已恢复。";
    }
    await refresh();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restoring.value.delete(v);
    restoring.value = new Map(restoring.value);
  }
}

async function updateFromSource() {
  const name = String(props.skill ?? "").trim();
  if (!name) return;
  if (!canUpdateFromSource.value) return;
  if (updatingFromSource.value) return;
  updatingFromSource.value = true;
  error.value = "";
  notice.value = "";
  try {
    const out = await updateSkillVersionBySkill(name);
    if (out.updated) {
      if (out.version?.id) {
        notice.value = `已拉取更新并生成快照：${out.version.id}。`;
      } else {
        notice.value = "已拉取更新并生成快照。";
      }
    } else {
      notice.value = "已是最新，无需生成新快照。";
    }
    await refresh();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    updatingFromSource.value = false;
  }
}
</script>

<template>
  <div v-show="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal skillVersionsModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">{{ title }}</div>
        <button
          v-if="sourceType === 'git'"
          type="button"
          class="headerMiniBtn"
          @click="updateFromSource"
          :disabled="loading || updatingFromSource || !canUpdateFromSource"
          title="从 Git 来源拉取并按需自动生成快照"
        >
          {{ updatingFromSource ? "更新中…" : "拉取更新" }}
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="refresh"
          :disabled="loading"
          title="刷新"
        >
          刷新
        </button>
        <button class="iconBtn" type="button" @click="emit('close')" aria-label="关闭">
          ✕
        </button>
      </div>

      <div class="modalBody">
        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-else-if="loading" class="loading">加载中…</div>
        <template v-else>
          <div v-if="notice" class="tinyHint">{{ notice }}</div>
          <div class="tinyHint" v-if="data">
            <div>
              版本目录：<span class="mono">{{ data.versions_root }}</span>
            </div>
            <div>
              技能来源：<span class="mono">{{ data.skill_source }}</span>
            </div>
            <div v-if="sourceType">
              来源类型：<span class="pill mono kind">{{ sourceType }}</span>
            </div>
            <div v-if="sourceRef">
              {{ sourceRefLabel }}：<span class="mono">{{ sourceRef }}</span>
            </div>
            <div v-if="sourceSubpath">
              Subpath：<span class="mono">{{ sourceSubpath }}</span>
            </div>
            <div v-if="sourceBranch">
              Branch：<span class="mono">{{ sourceBranch }}</span>
            </div>
            <div v-if="sourceRevision" :title="sourceRevision">
              Revision：<span class="mono">{{ shortRevision }}</span>
            </div>
          </div>

          <div v-if="!hasSource" class="tinyHint warn">
            该技能缺少来源（source）。仍可创建快照（将从当前可用的技能目录进行快照）；如需启用/同步，请先在 Skills 页添加来源。
          </div>

          <div class="skillsVersionsCreate">
            <input v-model="newId" placeholder="版本 ID（可选）例如：20260201-01" :disabled="!canCreate" />
            <input v-model="newNote" placeholder="备注（可选）" :disabled="!canCreate" />
            <button
              type="button"
              class="primary"
              @click="createFromForm"
              :disabled="creating || !canCreate"
            >
              {{ creating ? "创建中…" : "生成快照" }}
            </button>
          </div>
          <div class="tinyHint">
            版本 ID 留空会自动生成（例如 <span class="mono">YYYYMMDD-01</span>）。
          </div>

          <div class="skillsVersionsList">
            <div v-if="!versions.length" class="empty">暂无版本</div>
            <div v-else>
              <div v-for="v in versions" :key="v.id" class="skillsVersionRow">
                <div class="skillsVersionMain">
                  <div class="mono">{{ v.id }}</div>
                  <div class="tinyHint" v-if="v.note">{{ v.note }}</div>
                </div>
                <div class="skillsVersionRight">
                  <div class="tinyHint mono" v-if="v.created_at">{{ v.created_at }}</div>
                  <button
                    type="button"
                    class="warnBtn"
                    @click="restoreByID(v.id)"
                    :disabled="!!restoring.get(v.id)"
                    title="恢复到该版本"
                  >
                    {{ restoring.get(v.id) ? "…" : "恢复" }}
                  </button>
                  <button
                    type="button"
                    class="dangerBtn"
                    @click="deleteByID(v.id)"
                    :disabled="!!deleting.get(v.id)"
                    title="删除版本"
                  >
                    {{ deleting.get(v.id) ? "…" : "删除" }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
