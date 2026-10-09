# Local Dream frontend

React + TypeScript + Vite. The interface uses a light studio layout inspired by JiMeng: a persistent sidebar, a central prompt composer, progressive parameter disclosure, and reusable prompt starters. Illustrations are CSS artwork, not generated video previews.

## Run

Start the Go service from `app` with `go run .`. For frontend development, run `npm run dev` in this directory and open `http://127.0.0.1:5174`. API and uploaded media requests proxy to port 8790. Production uses `npm run build` and is served by Go at `http://127.0.0.1:8790`.

## Reading order for learning React

1. `src/main.tsx`: mount React and import the stylesheet.
2. `src/types.ts`: shared API contracts and component state types.
3. `src/components/Parameters.tsx`: a controlled component. Its parent owns the value; the child emits changes through a callback.
4. `src/components/Composer.tsx`: refs preserve cursor position, while state controls visible panels. Props connect it to the application.
5. `src/App.tsx`: compose the screens and coordinate user actions. State stays here when it must survive navigation between screens.
6. `src/hooks/useLibrary.ts`: effects load remote data and clean up timers/requests. A revision counter prevents old responses from replacing newer results.
7. `src/lib/api.ts`: the HTTP boundary handles timeouts, non-JSON responses, server errors, and null lists.
8. `src/lib/generation.ts`: pure helpers keep reference ordering, mode changes, and basic validation outside JSX.

## Component responsibilities

| Module                | Responsibility                                                    |
| --------------------- | ----------------------------------------------------------------- |
| Sidebar               | Navigation and local workspace identity                           |
| Composer + Parameters | Prompt editing, ordered references, mode and parameter selection  |
| Inspiration           | Local prompt starters; clicking never submits a generation        |
| AssetLibrary          | Asset filtering, media previews, references and deletion          |
| Jobs                  | Status filtering, progress, cancellation, reuse and download      |
| SettingsDialog        | Provider configuration, model list, save feedback and modal focus |

Styles use shared variables in `src/styles.css`, with layout, creator and library styles in `src/styles/`. Change tokens first when adjusting colors. Avoid adding a second override stylesheet. Responsive rules live beside the feature they modify.

## Behavior and boundaries

- Ctrl/Cmd + Enter submits a non-empty description. Failed submissions preserve the description and release the submission lock.
- `@filename` references use their order in the prompt, not their upload order. The same helper supplies reference chips and API asset IDs.
- Switching modes clears the acceleration LoRA. Selecting an acceleration preset controls its step count.
- Agent is optional and disabled initially. It uses the saved provider, sends description/asset information to that provider, and never auto-submits the resulting prompt.
- No hardcoded asset-count ceiling is displayed. The existing backend owns runtime/workflow limits; basic frontend validation only checks required reference presence.
- The connection badge checks Local Dream API availability, not ComfyUI model readiness.
- Assets and generation records currently belong to the running Go service session. This UI does not add backend persistence.
- Page navigation preserves the in-memory prompt. Reloading the browser does not preserve unsent drafts.

## Verification

### Parameter and mention interactions

`GenerationControls.tsx` renders compact format/duration triggers. Ratio silhouettes and resolution segments share a floating panel; duration has a separate slider and numeric control. The existing 5/10/15-second choices are retained. `Parameters.tsx` now owns only advanced sampling/LoRA settings.

`FloatingPanel.tsx` portals panels to the document body and keeps them inside the viewport. `lib/caret.ts` mirrors textarea typography to locate the caret after wrapping and scrolling. The mention list filters names, supports Arrow Up/Down and Enter, and inserts at the saved selection without removing the following text. Escape and outside clicks dismiss panels.

These interactions were checked against the live JiMeng video UI, then browser-tested at 1440px and 390px, including multiline Chinese input, insertion in the middle of a sentence, null/empty candidates, and textarea scrolling. No external generation request was submitted during this check.

Run `npm run typecheck`, `npm run build`, and `npm run format:check`.

Browser QA on 2026-09-10 covered the actual Go-served build, empty states, prompt starters, parameters and LoRA reset, missing-reference validation, sidebar navigation, filtering, settings opening/Escape, Agent toggling, and 390px/1440px layouts. Isolated browser response fixtures covered ordered references, submission failure/retry, queue rendering, cancellation, and prompt reuse. Fixtures do not demonstrate actual model inference or a working provider connection. No real H3 output was generated during the redesign.
