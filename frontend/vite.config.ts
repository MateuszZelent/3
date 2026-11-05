import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	resolve: {
		alias: {
			$lib: fileURLToPath(new URL('./src/lib', import.meta.url)),
			$api: fileURLToPath(new URL('./src/api', import.meta.url)),
			$components: fileURLToPath(new URL('./src/components', import.meta.url))
		}
	},
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			adapter: adapter({
				pages: 'dist',
				assets: 'dist',
				fallback: undefined,
				precompress: false,
				strict: true
			})
		})
	],
	build: {
		rolldownOptions: {
			output: {
				codeSplitting: {
					groups: [
						{ name: 'vendor-three', test: /node_modules[\\/]three[\\/]/ },
						{ name: 'vendor-zrender', test: /node_modules[\\/]zrender[\\/]/ },
						{ name: 'vendor-echarts', test: /node_modules[\\/]echarts[\\/]/ }
					]
				}
			}
		}
	},
	server: {
		proxy: {
			'/api': {
				target: 'http://localhost:35367',
				changeOrigin: true
			},
			'/ws': {
				target: 'ws://localhost:35367',
				ws: true
			}
		}
	}
});
