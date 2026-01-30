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
</script>

<template>
  <div class="skillsVersionsCard">
    <div class="skillsVersionsHeader">
      <div class="skillsVersionsTitle">Versions</div>
      <button type="button" class="headerMiniBtn" @click="emit('refresh')" :disabled="loading">
        Refresh
      </button>
    </div>

    <div v-if="error" class="modalError">{{ error }}</div>
    <div v-else-if="loading" class="loading">Loading...</div>
    <template v-else>
      <div class="skillsVersionsMeta tinyHint">
        <div>
          Root: <span class="mono">{{ data?.versions_root ?? "" }}</span>
        </div>
        <div>
          Source: <span class="mono">{{ data?.source_root ?? "" }}</span>
        </div>
      </div>

      <div class="skillsVersionsCreate">
        <input v-model="newIdModel" placeholder="version id (optional) e.g. 20260130-01" />
        <input v-model="newNoteModel" placeholder="note (optional)" />
        <button type="button" class="primary" @click="emit('create')" :disabled="creating">
          {{ creating ? "Creating..." : "Snapshot" }}
        </button>
      </div>
      <div class="tinyHint">
        Leave version id empty to auto-generate (e.g. <span class="mono">YYYYMMDD-01</span>).
      </div>

      <div class="skillsVersionsList">
        <div v-if="!versions.length" class="empty">No versions</div>
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
                @click="emit('delete', v.id)"
                :disabled="!!deleting.get(v.id)"
                title="Delete version"
              >
                {{ deleting.get(v.id) ? "..." : "Delete" }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

