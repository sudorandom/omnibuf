import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'astro/config';
import tailwindcss from 'tailwindcss';
import tailwindConfig from './tailwind.config.mjs';

function checkInternalLinksAndAnchors() {
  return {
    name: 'check-internal-links-and-anchors',
    hooks: {
      'astro:build:done': async ({ dir, logger }) => {
        const distDir = fileURLToPath(dir);

        async function collectHtmlFiles(currentDir) {
          const entries = await readdir(currentDir, { withFileTypes: true });
          const files = [];
          for (const entry of entries) {
            const fullPath = path.join(currentDir, entry.name);
            if (entry.isDirectory()) {
              files.push(...(await collectHtmlFiles(fullPath)));
            } else if (entry.isFile() && entry.name.endsWith('.html')) {
              files.push(fullPath);
            }
          }
          return files;
        }

        const htmlFiles = await collectHtmlFiles(distDir);
        const pageIds = new Map();
        const pageLinks = new Map();

        const normalizeRoute = (routePath) => {
          const clean = routePath.replace(/\/index\.html$/, '').replace(/\.html$/, '');
          if (!clean || clean === '/') return '/';
          return clean.endsWith('/') ? clean.slice(0, -1) : clean;
        };

        for (const file of htmlFiles) {
          const relPath = '/' + path.relative(distDir, file).split(path.sep).join('/');
          const route = normalizeRoute(relPath);
          const html = await readFile(file, 'utf8');

          const ids = new Set();
          for (const match of html.matchAll(/\bid=(?:"([^"]+)"|'([^']+)')/g)) {
            ids.add(match[1] ?? match[2]);
          }
          pageIds.set(route, ids);

          const hrefs = [];
          for (const match of html.matchAll(/<a\b[^>]*?\bhref=(?:"([^"]+)"|'([^']+)')/g)) {
            hrefs.push(match[1] ?? match[2]);
          }
          pageLinks.set(route, hrefs);
        }

        const broken = [];
        for (const [sourceRoute, hrefs] of pageLinks.entries()) {
          for (const href of hrefs) {
            if (
              !href ||
              href.startsWith('http://') ||
              href.startsWith('https://') ||
              href.startsWith('mailto:') ||
              href.startsWith('tel:')
            ) {
              continue;
            }

            let targetRoute = sourceRoute;
            let hash;

            if (href.startsWith('#')) {
              hash = href.slice(1);
            } else if (href.startsWith('/')) {
              const [rawPath, rawHash = ''] = href.split('#');
              targetRoute = normalizeRoute(rawPath || '/');
              hash = rawHash;
            } else {
              continue;
            }

            const targetIds = pageIds.get(targetRoute);
            if (!targetIds) {
              broken.push(
                `${sourceRoute}: broken page link href="${href}" (missing route ${targetRoute})`,
              );
              continue;
            }
            if (hash && !targetIds.has(hash)) {
              broken.push(
                `${sourceRoute}: broken anchor href="${href}" (#${hash} not found on ${targetRoute})`,
              );
            }
          }
        }

        if (broken.length > 0) {
          for (const err of broken) {
            logger.error(err);
          }
          throw new Error(`Found ${broken.length} broken internal link(s) or anchor(s).`);
        }

        logger.info(`Verified internal links and #anchors across ${htmlFiles.length} HTML pages.`);
      },
    },
  };
}

export default defineConfig({
  site: 'https://omnibuf.com',
  integrations: [checkInternalLinksAndAnchors()],
  vite: {
    css: {
      postcss: {
        plugins: [tailwindcss(tailwindConfig)],
      },
    },
  },
});
