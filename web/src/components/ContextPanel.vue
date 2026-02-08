<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import type { ProjectContext, PromptTemplate, PromptTemplateKind } from "../types";
import {
  deletePromptTemplate,
  fetchProjectContext,
  fetchPromptTemplates,
  setProjectContext,
  upsertPromptTemplate,
} from "../api";

const emit = defineEmits<{
  (e: "back"): void;
}>();

const contextLoading = ref(false);
const contextSaving = ref(false);
const contextError = ref("");
const contextDraft = ref("");
const contextSavedAt = ref<string>("");
const contextDirty = ref(false);

const templatesLoading = ref(false);
const templatesSaving = ref(false);
const templatesError = ref("");
const templatesKind = ref<PromptTemplateKind | "all">("all");
const templates = ref<PromptTemplate[]>([]);

const editorID = ref<string>("");
const editorTitle = ref<string>("");
const editorKind = ref<PromptTemplateKind>("task");
const editorContent = ref<string>("");

const editorDirty = ref(false);

const MAX_RUNES = 6000;

function normalizeContext(s: string): string {
  s = String(s ?? "").trim();
  if (!s) return "";
  s = s.replaceAll("\r\n", "\n").replaceAll("\r", "\n");
  const lines = s.split("\n");
  const out: string[] = [];
  let blankStreak = 0;
  for (const raw of lines) {
    const line = raw.replace(/\s+$/g, "");
    if (!line.trim()) {
      if (blankStreak >= 1) continue;
      out.push("");
      blankStreak++;
      continue;
    }
    blankStreak = 0;
    out.push(line);
  }
  return out.join("\n").trim();
}

function compressContext(raw: string, maxRunes: number): { text: string; truncated: boolean } {
  const normalized = normalizeContext(raw);
  if (!normalized || !maxRunes || maxRunes <= 0) return { text: normalized, truncated: false };
  const runes = Array.from(normalized);
  if (runes.length <= maxRunes) return { text: normalized, truncated: false };
  const keep = Math.max(1, maxRunes - 1);
  return { text: runes.slice(0, keep).join("") + "…", truncated: true };
}

const effectiveContext = computed(() => compressContext(contextDraft.value, MAX_RUNES));
const effectiveRunes = computed(() => Array.from(effectiveContext.value.text || "").length);

async function loadContext() {
  contextLoading.value = true;
  contextError.value = "";
  try {
    const got = await fetchProjectContext();
    contextDraft.value = String(got?.content ?? "");
    contextSavedAt.value = String(got?.updated_at ?? "");
    contextDirty.value = false;
  } catch (e: any) {
    contextError.value = e?.message ?? String(e);
  } finally {
    contextLoading.value = false;
  }
}

async function saveContext() {
  if (contextSaving.value) return;
  contextSaving.value = true;
  contextError.value = "";
  try {
    await setProjectContext(contextDraft.value);
    await loadContext();
  } catch (e: any) {
    contextError.value = e?.message ?? String(e);
  } finally {
    contextSaving.value = false;
  }
}

async function loadTemplates() {
  templatesLoading.value = true;
  templatesError.value = "";
  try {
    templates.value = await fetchPromptTemplates(templatesKind.value);
    // Keep selection stable if possible.
    if (editorID.value) {
      const hit = templates.value.find((t) => t.id === editorID.value);
      if (hit) fillEditor(hit);
    }
  } catch (e: any) {
    templatesError.value = e?.message ?? String(e);
  } finally {
    templatesLoading.value = false;
  }
}

function fillEditor(t: PromptTemplate) {
  editorID.value = String(t?.id ?? "");
  editorTitle.value = String(t?.title ?? "");
  editorKind.value = (t?.kind as PromptTemplateKind) ?? "task";
  editorContent.value = String(t?.content ?? "");
  editorDirty.value = false;
}

function newTemplate(kind?: PromptTemplateKind) {
  editorID.value = "";
  editorTitle.value = "";
  editorKind.value = kind ?? "task";
  editorContent.value = "";
  editorDirty.value = false;
}

async function saveTemplate() {
  if (templatesSaving.value) return;
  templatesSaving.value = true;
  templatesError.value = "";
  try {
    const tpl = await upsertPromptTemplate({
      id: editorID.value || undefined,
      title: editorTitle.value,
      kind: editorKind.value,
      content: editorContent.value,
    });
    editorID.value = tpl.id;
    editorDirty.value = false;
    await loadTemplates();
  } catch (e: any) {
    templatesError.value = e?.message ?? String(e);
  } finally {
    templatesSaving.value = false;
  }
}

async function deleteTemplate() {
  const id = String(editorID.value ?? "").trim();
  if (!id) return;
  if (!window.confirm("删除该模板？")) return;
  templatesSaving.value = true;
  templatesError.value = "";
  try {
    await deletePromptTemplate(id);
    newTemplate(editorKind.value);
    await loadTemplates();
  } catch (e: any) {
    templatesError.value = e?.message ?? String(e);
  } finally {
    templatesSaving.value = false;
  }
}

watch(
  () => templatesKind.value,
  () => {
    void loadTemplates();
  },
);

onMounted(() => {
  void loadContext();
  void loadTemplates();
});
</script>

<template>
  <div class="contextPanel">
    <div class="contextHeader">
      <div class="contextHeaderLeft">
        <div class="contextTitle">上下文 / 模板</div>
        <div class="contextSubtitle">Project Context 与 Prompt Templates（会压缩/限长）</div>
      </div>
      <span class="h2Spacer"></span>
      <button type="button" class="contextCloseBtn" @click="emit('back')" aria-label="Close">
        <span aria-hidden="true">×</span>
      </button>
    </div>

    <div v-if="contextError || templatesError" class="modalError">
      {{ contextError || templatesError }}
    </div>

    <div class="contextGrid">
      <section class="contextSection">
        <div class="contextSectionTitle">
          <div class="contextSectionTitleLeft">Project Context</div>
          <span class="h2Spacer"></span>
          <div class="contextSectionTitleActions">
            <button
              type="button"
              class="primary"
              @click="saveContext"
              :disabled="contextLoading || contextSaving || !contextDirty"
              :title="contextDirty ? '保存 Project Context' : '无更改'"
            >
              {{ contextSaving ? "保存中…" : "保存" }}
            </button>
          </div>
        </div>
        <textarea
          v-model="contextDraft"
          rows="14"
          placeholder="建议结构：角色 / 目标 / 约束 / DoD…（会注入到秘书与运行中；会做压缩/限长）"
          @input="contextDirty = true"
          spellcheck="false"
        ></textarea>
        <div class="tinyHint">
          有效长度：<span class="mono">{{ effectiveRunes }}</span> /
          <span class="mono">{{ MAX_RUNES }}</span>
          <span v-if="effectiveContext.truncated" class="warn">（将被截断）</span>
          <span v-if="contextSavedAt" class="mono"> · updated_at {{ contextSavedAt }}</span>
        </div>
        <div class="tinyHint">
          说明：系统会对上下文做确定性压缩（去多余空行/截断），不会使用 LLM 自动总结。
        </div>
      </section>

      <section class="contextSection">
        <div class="contextSectionTitle">
          <div class="contextSectionTitleLeft">Prompt Templates</div>
          <span class="h2Spacer"></span>
          <div class="contextSectionTitleActions">
            <button type="button" @click="loadTemplates" :disabled="templatesLoading || templatesSaving">刷新</button>
            <button type="button" class="primary" @click="newTemplate('task')" :disabled="templatesSaving">
              新建任务模板
            </button>
            <button type="button" class="primary" @click="newTemplate('chat')" :disabled="templatesSaving">
              新建对话模板
            </button>
          </div>
        </div>

        <div class="templatesToolbar">
          <label class="templatesKindFilter">
            <span class="fieldLabel">Kind</span>
            <select v-model="templatesKind" :disabled="templatesLoading || templatesSaving">
              <option value="all">all</option>
              <option value="task">task</option>
              <option value="chat">chat</option>
            </select>
          </label>
          <div class="tinyHint templatesHint">
            New Run 使用 <span class="mono">task</span> 模板；<span class="mono">chat</span> 模板保留为后续能力扩展。
          </div>
        </div>

        <div class="templatesBody">
          <div class="templatesList">
            <div v-if="templatesLoading" class="loading">加载中…</div>
            <div v-else-if="!templates.length" class="templatesEmpty">暂无模板</div>
            <button
              v-for="t in templates"
              :key="t.id"
              type="button"
              class="templatesItem"
              :class="{ active: t.id === editorID }"
              @click="fillEditor(t)"
              :title="t.content"
            >
              <div class="templatesItemTop">
                <span class="templatesItemTitle">{{ t.title }}</span>
                <span class="pill low mono">{{ t.kind }}</span>
              </div>
              <div v-if="t.updated_at" class="tinyHint mono">updated_at {{ t.updated_at }}</div>
            </button>
          </div>

          <div class="templatesEditor">
            <label class="templatesField templatesFieldTitle">
              <span class="fieldLabel">Title</span>
              <input v-model="editorTitle" placeholder="模板标题" @input="editorDirty = true" />
            </label>
            <label class="templatesField templatesFieldKind">
              <span class="fieldLabel">Kind</span>
              <select v-model="editorKind" @change="editorDirty = true" aria-label="Template kind">
                <option value="task">task</option>
                <option value="chat">chat</option>
              </select>
            </label>
            <label class="templatesField full">
              <span class="fieldLabel">Content</span>
              <textarea
                v-model="editorContent"
                rows="9"
                placeholder="模板内容…"
                @input="editorDirty = true"
                spellcheck="false"
              ></textarea>
            </label>
            <div class="templatesActions">
              <button type="button" class="dangerBtn" @click="deleteTemplate" :disabled="templatesSaving || !editorID">
                删除
              </button>
              <button
                type="button"
                class="primary"
                @click="saveTemplate"
                :disabled="templatesSaving || !editorTitle.trim() || !editorContent.trim() || !editorDirty"
              >
                {{ templatesSaving ? "保存中…" : "保存" }}
              </button>
            </div>
            <div class="tinyHint">
              提示：模板只会填充输入框；不会自动触发运行/发送。
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
