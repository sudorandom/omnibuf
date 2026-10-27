import { copyFile, readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import sitemap from '@astrojs/sitemap';
import { defineConfig } from 'astro/config';
import tailwindcss from 'tailwindcss';
import tailwindConfig from './tailwind.config.mjs';

const PAGE_LABELS = {
  '/': 'Overview',
  '/how-it-works': 'How It Works',
  '/spec': 'Specification',
  '/go': 'Go',
  '/es': 'TypeScript / JS',
};

const PAGE_ORDER = ['/', '/how-it-works', '/spec', '/go', '/es'];

function decodeEntities(str) {
  return str
    .replace(/&rarr;/g, '→')
    .replace(/&larr;/g, '←')
    .replace(/&hellip;/g, '…')
    .replace(/&bull;/g, '•')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&#(\d+);/g, (_, n) => String.fromCharCode(Number(n)));
}

function cleanHtmlText(html) {
  return decodeEntities(
    html
      .replace(/<a\b[^>]*class="[^"]*spec-anchor[^"]*"[^>]*>[\s\S]*?<\/a>/gi, ' ')
      .replace(/<button\b[^>]*class="[^"]*copy-snippet-btn[^"]*"[^>]*>[\s\S]*?<\/button>/gi, ' ')
      .replace(/<[^>]+>/g, ' '),
  )
    .replace(/\s+/g, ' ')
    .trim();
}

function extractSearchEntries(route, html) {
  const page = PAGE_LABELS[route];
  if (!page) return [];
  const mainMatch = html.match(/<main\b[^>]*>([\s\S]*?)<\/main>/i);
  if (!mainMatch) return [];

  const mainHtml = mainMatch[1]
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, ' ')
    .replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, ' ')
    .replace(/<svg\b[^>]*>[\s\S]*?<\/svg>/gi, ' ');

  const headingRegex = /<(h[1-3])\b([^>]*)>([\s\S]*?)<\/\1>/gi;
  const matches = [...mainHtml.matchAll(headingRegex)];
  const entries = [];
  let currentH2Title = '';
  let currentH2Id = '';

  for (let i = 0; i < matches.length; i++) {
    const m = matches[i];
    const tag = m[1].toLowerCase();
    const attrs = m[2];
    const title = cleanHtmlText(m[3]).replace(/\.$/, '');

    let id = (attrs.match(/\bid="([^"]+)"/i) || [])[1] || '';
    if (!id && tag !== 'h1') {
      const prevStart = Math.max(0, m.index - 450);
      const beforeSlice = mainHtml.slice(prevStart, m.index);
      const containerIds = [...beforeSlice.matchAll(/<(?:section|div)\b[^>]*?\bid="([^"]+)"/gi)];
      if (containerIds.length > 0) {
        id = containerIds[containerIds.length - 1][1];
      } else if (tag === 'h3' && currentH2Id) {
        id = currentH2Id;
      }
    }

    if (tag === 'h2') {
      currentH2Title = title;
      currentH2Id = id;
    }

    const bodyStart = m.index + m[0].length;
    const bodyEnd = i + 1 < matches.length ? matches[i + 1].index : mainHtml.length;
    const text = cleanHtmlText(mainHtml.slice(bodyStart, bodyEnd));
    const url = id ? `${route === '/' ? '/' : route}#${id}` : route;

    entries.push({
      page,
      section: tag === 'h3' ? currentH2Title : '',
      title,
      url,
      text,
    });
  }

  return entries;
}

function checkInternalLinksAndAnchors() {
  return {
    name: 'check-internal-links-and-anchors',
    hooks: {
      'astro:server:setup': ({ server }) => {
        server.middlewares.use(async (req, res, next) => {
          const url = req.url?.split('?')[0];
          if (url === '/search-index.json') {
            try {
              const host = req.headers.host || 'localhost:4321';
              const entries = [];
              for (const route of PAGE_ORDER) {
                const resp = await globalThis.fetch(`http://${host}${route}`);
                if (resp.ok) {
                  const html = await resp.text();
                  entries.push(...extractSearchEntries(route, html));
                }
              }
              res.statusCode = 200;
              res.setHeader('Content-Type', 'application/json; charset=utf-8');
              res.end(JSON.stringify(entries));
              return;
            } catch (err) {
              res.statusCode = 500;
              res.setHeader('Content-Type', 'application/json; charset=utf-8');
              res.end(JSON.stringify({ error: String(err) }));
              return;
            }
          }
          if (url === '/sitemap.xml' || url === '/sitemap-0.xml' || url === '/sitemap-index.xml') {
            const pagesDir = path.join(path.dirname(fileURLToPath(import.meta.url)), 'src/pages');
            const entries = await readdir(pagesDir);
            const urls = entries
              .filter((name) => name.endsWith('.astro') && name !== '404.astro')
              .map((name) => (name === 'index.astro' ? '/' : `/${name.replace(/\.astro$/, '')}/`))
              .sort()
              .map((route) => `<url><loc>https://omnibuf.com${route}</loc></url>`)
              .join('');
            const xml =
              url === '/sitemap-index.xml'
                ? `<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://omnibuf.com/sitemap-0.xml</loc></sitemap></sitemapindex>`
                : `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:news="http://www.google.com/schemas/sitemap-news/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml" xmlns:image="http://www.google.com/schemas/sitemap-image/1.1" xmlns:video="http://www.google.com/schemas/sitemap-video/1.1">${urls}</urlset>`;
            res.statusCode = 200;
            res.setHeader('Content-Type', 'application/xml; charset=utf-8');
            res.end(xml);
            return;
          }
          next();
        });
      },
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
        const pageHtmlByRoute = new Map();

        const normalizeRoute = (routePath) => {
          const clean = routePath.replace(/\/index\.html$/, '').replace(/\.html$/, '');
          if (!clean || clean === '/') return '/';
          return clean.endsWith('/') ? clean.slice(0, -1) : clean;
        };

        for (const file of htmlFiles) {
          const relPath = '/' + path.relative(distDir, file).split(path.sep).join('/');
          const route = normalizeRoute(relPath);
          const html = await readFile(file, 'utf8');
          pageHtmlByRoute.set(route, html);

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

        const searchEntries = [];
        for (const route of PAGE_ORDER) {
          const html = pageHtmlByRoute.get(route);
          if (html) {
            searchEntries.push(...extractSearchEntries(route, html));
          }
        }
        const searchIndexPath = path.join(distDir, 'search-index.json');
        await writeFile(searchIndexPath, JSON.stringify(searchEntries), 'utf8');
        logger.info(`Generated search-index.json with ${searchEntries.length} sections.`);

        const sitemapZero = path.join(distDir, 'sitemap-0.xml');
        const sitemapXml = path.join(distDir, 'sitemap.xml');
        await copyFile(sitemapZero, sitemapXml);
      },
    },
  };
}

export default defineConfig({
  site: 'https://omnibuf.com',
  integrations: [sitemap(), checkInternalLinksAndAnchors()],
  vite: {
    css: {
      postcss: {
        plugins: [tailwindcss(tailwindConfig)],
      },
    },
  },
});
