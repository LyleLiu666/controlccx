<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { PerSkillVersionsListResponse } from "../types";
import {
  createSkillVersionBySkill,
  deleteSkillVersionBySkill,
  fetchSkillVersionsBySkill,
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

const title = computed(() => {
  const name = String(props.skill ?? "").trim();
  if (!name) return "版本快照";
  return `版本快照：${name}`;
});
const versions = computed(() => data.value?.versions ?? []);
const canCreate = computed(() => !!String(props.skill ?? "").trim() && props.hasSource);

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
</script>

<template>
  <div v-show="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal skillVersionsModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">{{ title }}</div>
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
          <div class="tinyHint" v-if="data">
            <div>
              版本目录：<span class="mono">{{ data.versions_root }}</span>
            </div>
            <div>
              技能来源：<span class="mono">{{ data.skill_source }}</span>
            </div>
          </div>

          <div v-if="!hasSource" class="modalError">
            该技能缺少来源（source），无法创建快照。请先在 Skills 页导入/安装该技能后再试。
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

