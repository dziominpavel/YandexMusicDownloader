// Статическая проверка заставки: разметка, стили и fail-open в app.js.
// Не подменяет визуальный прогон — только фиксирует контракт слоя.
const fs = require('fs');
const path = require('path');

const dist = path.join(__dirname, '..', 'src', 'frontend', 'dist');
const html = fs.readFileSync(path.join(dist, 'index.html'), 'utf8');
const css = fs.readFileSync(path.join(dist, 'app.css'), 'utf8');
const js = fs.readFileSync(path.join(dist, 'app.js'), 'utf8');

let failures = 0;
function check(name, cond, extra) {
  if (cond) console.log('  ok - ' + name);
  else { failures++; console.log('  FAIL - ' + name + (extra ? ' | ' + extra : '')); }
}

console.log('scenario: boot markup');
const bootStart = html.indexOf('id="boot"');
const appStart = html.indexOf('class="app-shell"');
const bootBlock = bootStart === -1 ? '' : html.slice(bootStart, appStart);
check('слой #boot есть', bootStart !== -1);
check('#boot стоит до .app-shell', bootStart !== -1 && appStart !== -1 && bootStart < appStart);
check('aria-hidden', /id="boot"[^>]*aria-hidden="true"/.test(html));
check('нет фокусируемых элементов', !/<(button|input|a|select|textarea)\b/i.test(bootBlock));
check('винил и wordmark', /icon-vinyl/.test(bootBlock) && /Yandex Music/.test(bootBlock) && /Downloader/.test(bootBlock));

console.log('scenario: boot css');
check('pointer-events: none', /\.boot\s*\{[^}]*pointer-events:\s*none/s.test(css));
check('failsafe', /@keyframes\s+boot-failsafe/.test(css));
check('вход диска', /@keyframes\s+boot-in/.test(css));
check('вращение', /@keyframes\s+boot-spin/.test(css));
check('wordmark', /@keyframes\s+boot-word/.test(css));
check('reduced motion', /prefers-reduced-motion[\s\S]{0,400}\.boot/.test(css));

console.log('scenario: boot js');
check('boot--run', /boot--run/.test(js));
check('минимальная длительность', /BOOT_MIN_MS/.test(js));
check('fail-open без setTimeout', /typeof setTimeout !== 'function'/.test(js));
check('isBootDismissed', /isBootDismissed/.test(js));

console.log(failures ? `\n${failures} failure(s)` : '\nall checks passed');
process.exit(failures ? 1 : 0);
