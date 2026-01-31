<script setup lang="ts">
import { computed } from "vue";

type FileNode = {
  name: string;
  path: string;
  kind: "dir" | "file";
  size?: number;
  expanded?: boolean;
  children?: FileNode[];
  loading?: boolean;
};

type VisibleNode = { node: FileNode; depth: number };

const props = defineProps<{
  root: FileNode | null;
  loading: boolean;
  error: string;
  notice: string;
  sidebarWidth: number;
  resizing: boolean;
  visibleNodes: VisibleNode[];
  selectedPath: string;
  selectedKind: "" | "dir" | "file";
  view: "preview" | "edit";
  dirty: boolean;
  fileSize: number;
  fileTruncated: boolean;
  fileContent: string;
  fileError: string;
  fileLoading: boolean;
  saving: boolean;
  isMarkdown: boolean;
  previewHtml: string;
  codeHtml: string;
  normalizePathForCompare: (p: string) => string;
}>();

const emit = defineEmits<{
  (e: "back"): void;
  (e: "refreshRoot"): void;
  (e: "newFile"): void;
  (e: "newFolder"): void;
  (e: "deleteSelected"): void;
  (e: "refreshDir"): void;
  (e: "nodeClick", node: FileNode): void;
  (e: "startResize", ev: MouseEvent): void;
  (e: "copy", text: string): void;
  (e: "save"): void;
  (e: "markdownClick", ev: MouseEvent): void;
  (e: "update:view", value: "preview" | "edit"): void;
  (e: "update:fileContent", value: string): void;
}>();

const viewModel = computed({
  get: () => props.view,
  set: (value: "preview" | "edit") => emit("update:view", value),
});
const fileContentModel = computed({
  get: () => props.fileContent,
  set: (value: string) => emit("update:fileContent", value),
});

function isActiveNode(path: string): boolean {
  return (
    props.normalizePathForCompare(path) ===
    props.normalizePathForCompare(props.selectedPath)
  );
}
</script>

<template>
  <section class="panel filesPagePanel">
    <div class="filesPageBody">
      <div class="filesTopRow">
        <div class="mono filesRootPath" :title="root?.path">
          {{ root?.path || (loading ? "Loading…" : "") }}
        </div>
        <div class="filesTopActions">
          <button
            type="button"
            class="filesTopBtn"
            @click="emit('refreshRoot')"
            :disabled="loading || !root"
          >
            Refresh
          </button>
          <button type="button" class="filesTopBtn" @click="emit('back')">
            Back
          </button>
        </div>
      </div>
      <div v-if="notice" class="tinyHint">{{ notice }}</div>

      <div v-if="error" class="modalError">
        {{ error }}
      </div>
      <div v-else-if="loading" class="loading">Loading...</div>
      <template v-else>
        <div class="filesSplit" :style="{ '--sidebar-width': sidebarWidth + 'px' }">
          <div class="filesTreePane">
            <div class="filesTreeActions">
              <button type="button" @click="emit('newFile')" :disabled="!root">
                New file
              </button>
              <button type="button" @click="emit('newFolder')" :disabled="!root">
                New folder
              </button>
              <button
                type="button"
                @click="emit('deleteSelected')"
                :disabled="!selectedPath || selectedPath === root?.path"
              >
                Delete
              </button>
            </div>

            <div class="filesTreeList">
              <button
                v-for="v in visibleNodes"
                :key="v.node.path"
                type="button"
                class="filesNode"
                :class="{ active: isActiveNode(v.node.path) }"
                :style="{ paddingLeft: `${12 + v.depth * 14}px` }"
                @click="emit('nodeClick', v.node)"
              >
                <span class="filesNodeTwisty">{{
                  v.node.kind === "dir" ? (v.node.expanded ? "▼" : "▶") : ""
                }}</span>
                <span class="filesNodeIcon">{{
                  v.node.kind === "dir" ? "📁" : "📄"
                }}</span>
                <span class="filesNodeName">{{ v.node.name }}</span>
                <span v-if="v.node.kind === 'file'" class="filesNodeMeta mono">{{
                  v.node.size ?? 0
                }}</span>
                <span v-if="v.node.loading" class="filesNodeMeta tinyHint">…</span>
              </button>
              <div v-if="!visibleNodes.length" class="empty">Empty folder</div>
            </div>
          </div>

          <div
            class="filesResizer"
            @mousedown="emit('startResize', $event)"
            :class="{ resizing }"
          ></div>

          <div class="filesEditorPane">
            <div v-if="selectedKind !== 'file'" class="empty">
              {{
                selectedKind === "dir"
                  ? "Select a file to preview/edit."
                  : "Select a file."
              }}
            </div>
            <template v-else>
              <div class="filesEditorHeader">
                <div class="mono filesEditorPath" :title="selectedPath">
                  {{ selectedPath }}
                </div>
                <span class="tinyHint mono">{{ fileSize }} bytes</span>
                <span v-if="fileTruncated" class="pill warn">truncated</span>
                <div class="tabSpacer"></div>
                <button
                  type="button"
                  @click="emit('copy', fileContent)"
                  :disabled="!fileContent"
                >
                  Copy
                </button>
                <button
                  type="button"
                  class="primary"
                  @click="emit('save')"
                  :disabled="saving || !dirty || fileTruncated"
                >
                  {{ saving ? "Saving..." : "Save" }}
                </button>
              </div>

              <div class="outputTabs">
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: viewModel === 'preview' }"
                  @click="viewModel = 'preview'"
                >
                  Preview
                </button>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: viewModel === 'edit' }"
                  @click="viewModel = 'edit'"
                >
                  Edit
                </button>
                <div class="tabSpacer"></div>
                <div v-if="dirty" class="tinyHint">unsaved</div>
              </div>

              <div v-if="fileError" class="modalError">
                {{ fileError }}
              </div>
              <div v-else-if="fileLoading" class="loading">Loading...</div>
              <template v-else>
                <div v-if="viewModel === 'edit'" class="filesEditorEdit">
                  <textarea
                    v-model="fileContentModel"
                    rows="18"
                    spellcheck="false"
                  ></textarea>
                </div>
                <template v-else>
                  <div
                    v-if="isMarkdown"
                    class="resultBox markdown filePreviewBox"
                    v-html="previewHtml"
                    @click="emit('markdownClick', $event)"
                  ></div>

                  <div v-else class="resultBox fileCodeBox">
                    <pre class="hljs"><code v-html="codeHtml"></code></pre>
                  </div>
                </template>
              </template>
            </template>
          </div>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
/* Note: App.vue styles are scoped and don't reach into this component.
   Keep Files page layout styles co-located here. */

.filesPagePanel {
  grid-column: 1 / -1;
  max-height: calc(100vh - 110px);
  max-height: calc(100dvh - 110px);
}

.filesPageBody {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.filesTopRow {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.filesTopActions {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.filesTopBtn {
  border-radius: 999px;
  padding: 6px 10px;
  font-weight: 800;
  font-size: 12px;
}

.filesRootPath {
  font-size: 12px;
  color: var(--text-sub);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.filesSplit {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: var(--sidebar-width, 340px) 12px 1fr;
  gap: 0;
}

.filesResizer {
  width: 100%;
  cursor: col-resize;
  background: transparent;
  display: flex;
  justify-content: center;
  transition: background 0.2s;
}

.filesResizer:hover,
.filesResizer.resizing {
  background: rgba(148, 163, 184, 0.25);
}

.filesTreePane,
.filesEditorPane {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: rgba(0, 0, 0, 0.02);
  padding: 10px;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

:global(:root[data-theme="dark"]) .filesTreePane,
:global(:root[data-theme="dark"]) .filesEditorPane {
  background: rgba(255, 255, 255, 0.03);
}

.filesTreeActions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.filesTreeList {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 4px 2px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.filesNode {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
  border-radius: 10px;
  padding: 8px 10px;
}

.filesNode:hover {
  background: rgba(13, 148, 136, 0.08);
}

.filesNode.active {
  background: var(--color-primary-bg);
  border: 1px solid rgba(13, 148, 136, 0.35);
}

.filesNodeTwisty {
  width: 14px;
  color: var(--text-sub);
  flex: 0 0 auto;
}

.filesNodeIcon {
  width: 18px;
  flex: 0 0 auto;
}

.filesNodeName {
  flex: 1 1 auto;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.filesNodeMeta {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--text-sub);
}

.filesEditorHeader {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.filesEditorPath {
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--text-sub);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1 1 auto;
  min-width: 0;
}

.filesEditorEdit {
  flex: 1;
  min-height: 0;
  display: flex;
}

.filesEditorEdit textarea {
  flex: 1;
  min-height: 0;
  resize: none;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
}

.outputTabs {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
  flex: 0 0 auto;
}

.tabBtn {
  padding: 6px 12px;
  border-radius: 999px;
}

.tabBtn.active {
  border-color: var(--color-primary);
  background: var(--color-primary-bg);
  color: var(--color-primary);
}

.tabSpacer {
  flex: 1;
}

.resultBox {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: 16px;
  background: var(--bg-panel);
  color: var(--text-main);
  flex: 1;
  overflow: auto;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  font-family: var(--font-main);
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.03);
}

.filePreviewBox,
.fileCodeBox {
  flex: 1;
  min-height: 0;
}

.fileCodeBox {
  white-space: normal;
  padding: 0;
}

.fileCodeBox pre {
  margin: 0;
  padding: 14px 16px;
  min-height: 100%;
  overflow: auto;
  background: transparent;
}

@media (max-width: 860px) {
  .filesSplit {
    grid-template-columns: 1fr;
  }
  .filesTreePane {
    max-height: 240px;
  }
  .filesResizer {
    display: none;
  }
}
</style>
