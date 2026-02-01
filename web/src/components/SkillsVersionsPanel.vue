<script setup lang="ts">
import { computed } from "vue";
import type { SkillVersionsListResponse } from "../types";

const props = defineProps<{
  loading: boolean;
  error: string;
  data: SkillVersionsListResponse | null;
  newId: string;
  newNote: string;
  creating: boolean;
  deleting: Map<string, boolean>;
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
  (e: "create"): void;
  (e: "delete", id: string): void;
  (e: "update:newId", value: string): void;
  (e: "update:newNote", value: string): void;
}>();

const newIdModel = computed({
  get: () => props.newId,
  set: (value: string) => emit("update:newId", value),
});
const newNoteModel = computed({
  get: () => props.newNote,
  set: (value: string) => emit("update:newNote", value),
});

const versions = computed(() => props.data?.versions ?? []);

function confirmDelete(id: string) {
  const v = String(id ?? "").trim();
  if (!v) return;
  if (!window.confirm(`确认删除版本「${v}」？此操作不可撤销。`)) return;
  emit("delete", v);
}
</script>

<template>
  <div class="skillsVersionsCard">
    <div class="skillsVersionsHeader">
      <div class="skillsVersionsTitle">版本快照</div>
      <button type="button" class="headerMiniBtn" @click="emit('refresh')" :disabled="loading">
        刷新
      </button>
    </div>

    <div v-if="error" class="modalError">{{ error }}</div>
    <div v-else-if="loading" class="loading">加载中…</div>
    <template v-else>
      <div class="skillsVersionsMeta tinyHint">
        <div>
          根目录：<span class="mono">{{ data?.versions_root ?? "" }}</span>
        </div>
        <div>
          来源：<span class="mono">{{ data?.source_root ?? "" }}</span>
        </div>
      </div>

      <div class="skillsVersionsCreate">
        <input v-model="newIdModel" placeholder="版本 ID（可选）例如：20260130-01" />
        <input v-model="newNoteModel" placeholder="备注（可选）" />
        <button type="button" class="primary" @click="emit('create')" :disabled="creating">
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
                @click="confirmDelete(v.id)"
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
</template>
