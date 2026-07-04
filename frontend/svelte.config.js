import preprocess from "svelte-preprocess";

// Svelte 4 + vite-plugin-svelte: TypeScript in <script lang="ts"> is handled
// by svelte-preprocess (esbuild-based type stripping).
export default {
  preprocess: [
    preprocess({
      typescript: true,
    }),
  ],
};
