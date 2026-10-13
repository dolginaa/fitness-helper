'use strict';

// The Mini App. It talks to the same use cases as the chat through the JSON API,
// so a workout started here can be continued in the chat and the other way round.

const app = document.getElementById('app');
const toastBox = document.getElementById('toast');

// Telegram passes launch parameters in the URL fragment. The fragment is never changed
// (navigation lives in history.state), so the Telegram script can read it whenever it loads.
const launch = new URLSearchParams(location.hash.slice(1));
const initData = launch.get('tgWebAppData') || sessionStorage.getItem('initData') || '';
if (initData) sessionStorage.setItem('initData', initData);
applyTheme(launch.get('tgWebAppThemeParams'));

let tg = null;

// ---------- formatting

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const num = (v) => String(Math.round(v * 100) / 100);
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);
const kindOf = (s) => s.kind || 'weight';
const kindNames = { weight: 'Вес', bodyweight: 'Свой вес', band: 'Резинка' };
const kindOrder = ['weight', 'bodyweight', 'band'];
const MAX_BAND = 10;

// A set in a list: 10×50, 15×св.вес, 15×св.вес+5, 12×рез.4 (the same as in the chat).
function setText(s) {
  switch (kindOf(s)) {
    case 'bodyweight': return `${s.reps}×св.вес${s.weight > 0 ? '+' + num(s.weight) : ''}`;
    case 'band': return `${s.reps}×рез.${s.band}`;
    default: return `${s.reps}×${num(s.weight)}`;
  }
}

// A set written out on the screen of an exercise.
function setLine(s) {
  switch (kindOf(s)) {
    case 'bodyweight': return `${s.reps} × свой вес${s.weight > 0 ? ' +' + num(s.weight) + ' кг' : ''}`;
    case 'band': return `${s.reps} × резинка ${s.band}`;
    default: return `${s.reps} × ${num(s.weight)} кг`;
  }
}

const setsText = (sets) => (sets.length ? sets.map(setText).join(', ') : 'нет данных');
const thousands = (v) => Math.round(v).toLocaleString('ru-RU');
const shortDate = (iso) => new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit' }).format(new Date(iso));
const longDate = (iso) => cap(new Intl.DateTimeFormat('ru-RU', { weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(iso)));

function applyTheme(json) {
  if (!json) return;
  try {
    for (const [key, value] of Object.entries(JSON.parse(json))) {
      document.documentElement.style.setProperty('--tg-theme-' + key.replace(/_/g, '-'), value);
    }
  } catch (_) { /* the theme is optional */ }
}

function toast(text) {
  toastBox.textContent = text;
  toastBox.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => { toastBox.hidden = true; }, 2600);
}

// ---------- API

async function request(method, path, body) {
  const headers = {};
  if (initData) headers.Authorization = 'tma ' + initData;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  return fetch('api/' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
}

async function api(method, path, body) {
  const res = await request(method, path, body);
  if (res.ok) return res.json();
  let code = 'internal';
  try { code = (await res.json()).error || code; } catch (_) { /* not JSON */ }
  const err = new Error(code);
  err.code = code;
  throw err;
}

const messages = {
  unauthorized: 'Откройте приложение из чата с ботом.',
  invalid_input: 'Проверьте введённые значения.',
  not_found: 'Запись не найдена.',
  internal: 'Что-то пошло не так. Попробуйте ещё раз.',
};
const messageFor = (err) => messages[err.code] || (err.code ? messages.internal : 'Нет связи с сервером.');

// Chart images need the same authorization as the API, so they are fetched, not linked.
async function loadCharts() {
  for (const img of app.querySelectorAll('img[data-chart]')) {
    try {
      const res = await request('GET', img.dataset.chart);
      if (!res.ok) continue;
      img.src = URL.createObjectURL(await res.blob());
      img.hidden = false;
    } catch (_) { /* a chart is a bonus */ }
  }
}

// ---------- navigation

// Screens are kept in history.state; depth is how far the screen is from home.
function current() {
  return history.state || { screen: 'home', params: {}, depth: 0 };
}

function go(screen, params = {}) {
  history.pushState({ screen, params, depth: current().depth + 1 }, '');
  render();
}

function replace(screen, params = {}) {
  history.replaceState({ screen, params, depth: current().depth }, '');
  render();
}

function goHome() {
  const { depth } = current();
  if (depth > 0) history.go(-depth); else replace('home');
}

window.addEventListener('popstate', render);

let renderSeq = 0;

async function render() {
  const seq = ++renderSeq;
  const { screen, params, depth } = current();
  if (tg) (depth > 0 ? tg.BackButton.show() : tg.BackButton.hide());
  if (!app.firstChild) app.innerHTML = '<div class="loading">Загрузка…</div>';
  try {
    const html = await screens[screen](params);
    if (seq !== renderSeq) return;
    app.innerHTML = (depth > 0 ? '<button class="back" data-act="back">‹ Назад</button>' : '') + html;
    window.scrollTo(0, 0);
    loadCharts();
  } catch (err) {
    if (seq !== renderSeq) return;
    if (err.code === 'no_active_workout' && depth > 0) { goHome(); return; }
    app.innerHTML = `<div class="card empty">${esc(messageFor(err))}</div>` +
      (err.code === 'unauthorized' ? '' : '<button class="btn quiet" data-act="retry">Повторить</button>');
  }
}

// ---------- screens

const chev = '<span class="chev" aria-hidden="true">›</span>';

// The numbers of a workout; with the report of a finished one also its duration and how it compares
// to the previous workout. Tonnage is left out when nothing was lifted (bands, the body).
function stats(w, r) {
  const items = [[w.exercises.length, 'упражнений'], [w.setCount, 'подходов']];
  if (r) items.push([r.reps, 'повторов']);
  if (w.volume > 0) items.push([thousands(w.volume), 'кг тоннаж']);
  if (r && r.durationMin > 0) items.push([duration(r.durationMin), 'длительность']);
  if (r && w.volume > 0 && r.prevVolume > 0) {
    const p = percent(r.prevVolume, w.volume);
    items.push([p === '0%' ? '=' : p, 'тоннаж к прошлой']);
  }
  return `<div class="card pad stats">${items.map(([value, label]) =>
    `<div class="stat"><b>${value}</b><span>${label}</span></div>`).join('')}</div>`;
}

function groupTiles(groups, screen) {
  return `<div class="grid">${groups.map((g) =>
    `<button class="tile" data-go="${screen}" data-id="${g.id}">${esc(g.name)}</button>`).join('')}</div>`;
}

const screens = {
  async home() {
    const { active } = await api('GET', 'home');
    const workout = active
      ? `<div class="section">Идёт тренировка</div>
         ${stats(active)}
         <button class="btn" data-go="workout">Продолжить тренировку</button>
         <button class="btn quiet" data-act="new-workout" data-force="1" data-confirm="Текущая завершится. Нажмите ещё раз">Завершить и начать новую</button>`
      : '<button class="btn" data-act="new-workout">Начать тренировку</button>';
    return `<h1>Тренировки</h1>
      <p class="sub">Подходы, веса и прогресс</p>
      ${workout}
      <div class="section">Разделы</div>
      <div class="card">
        <button class="row" data-go="history"><span class="main">История</span>${chev}</button>
        <button class="row" data-go="progress"><span class="main">Прогресс</span>${chev}</button>
      </div>`;
  },

  async workout() {
    const w = await api('GET', 'workout');
    const rows = w.exercises.map((ex) =>
      `<button class="row" data-go="exercise" data-id="${ex.id}">
         <span class="main"><div class="title">${esc(ex.name)}</div><div class="desc">${setsText(ex.sets)}</div></span>${chev}
       </button>`).join('');
    return `<h1>Тренировка</h1>
      <p class="sub">${longDate(w.date)}</p>
      ${w.exercises.length ? `<div class="card">${rows}</div>${stats(w)}` : '<div class="card empty">Пока нет упражнений — добавьте первое.</div>'}
      <button class="btn" data-go="groups">Добавить упражнение</button>
      <button class="btn danger" data-act="finish" data-confirm="Завершить? Нажмите ещё раз">Завершить тренировку</button>`;
  },

  async groups() {
    const groups = await api('GET', 'groups');
    return `<h1>Группа мышц</h1><p class="sub">Что тренируем?</p>${groupTiles(groups, 'group')}`;
  },

  async group({ id }) {
    const list = await api('GET', `groups/${id}/exercises?workout=1`);
    const rows = list.exercises.map((ex) => {
      const done = ex.todaySets.length > 0;
      const desc = done ? 'Сегодня: ' + setsText(ex.todaySets)
        : ex.lastAt ? `${shortDate(ex.lastAt)}: ${setsText(ex.lastSets)}` : 'Ещё не делали';
      return `<button class="row" data-go="exercise" data-id="${ex.id}">
          <span class="mark" aria-hidden="true">${done ? '✓' : ''}</span>
          <span class="main"><div class="title">${esc(ex.name)}</div><div class="desc">${desc}</div></span>${chev}
        </button>`;
    }).join('');
    return `<h1>${esc(cap(list.group.name))}</h1>
      <p class="sub">Выберите упражнение</p>
      ${rows ? `<div class="card">${rows}</div>` : '<div class="card empty">В этой группе пока нет упражнений.</div>'}
      <div class="section">Нет в списке?</div>
      <form data-form="new-exercise" data-id="${id}">
        <input class="field" name="name" maxlength="100" placeholder="Название своего упражнения" autocomplete="off">
        <div class="card checks">${kindOrder.map((k, i) => checkRow(k, i === 0)).join('')}</div>
        <button class="btn quiet">Добавить упражнение</button>
      </form>`;
  },

  async exercise({ id }) {
    return exerciseView(await api('POST', `exercises/${id}/begin`));
  },

  async loads({ id }) {
    const log = await api('POST', `exercises/${id}/begin`);
    const on = new Set(log.exercise.loads);
    return `<h1>${esc(log.exercise.name)}</h1>
      <p class="sub">С чем вы делаете это упражнение? Отметьте всё, что подходит: если вариантов несколько, на экране подхода появится переключатель. Это общая настройка упражнения.</p>
      <div class="card checks" data-loads="${id}">${kindOrder.map((k) => checkRow(k, on.has(k))).join('')}</div>
      <button class="btn" data-act="back">Готово</button>`;
  },

  async finished({ result }) {
    if (!result.saved) {
      return `<h1>Тренировка не сохранена</h1><p class="sub">В ней не было ни одного подхода.</p>
        <button class="btn" data-act="home">На главную</button>`;
    }
    return `<h1>Тренировка завершена</h1><p class="sub">Хорошая работа!</p>
      ${stats(result.workout, result.report)}${result.report ? reportHtml(result.report) : ''}${workoutCards(result.workout)}
      <button class="btn" data-act="home">На главную</button>`;
  },

  async history({ pages = 1 }) {
    const items = [];
    let hasNext = false;
    for (let page = 0; page < pages; page++) {
      const p = await api('GET', `history?page=${page}`);
      items.push(...p.items);
      hasNext = p.hasNext;
    }
    if (!items.length) {
      return '<h1>История</h1><div class="card empty">Завершённых тренировок пока нет.</div>';
    }
    const rows = items.map((w) =>
      `<button class="row" data-go="historyWorkout" data-id="${w.id}">
         <span class="main"><div class="title">${longDate(w.date)}</div>
         <div class="desc">${esc(w.groups.join(', '))} · упражнений: ${w.exercises}</div></span>${chev}
       </button>`).join('');
    return `<h1>История</h1><p class="sub">Завершённые тренировки</p>
      <div class="card">${rows}</div>
      ${hasNext ? `<button class="btn quiet" data-act="more-history" data-pages="${pages + 1}">Показать ещё</button>` : ''}`;
  },

  async historyWorkout({ id }) {
    const w = await api('GET', `workouts/${id}`);
    return `<h1>Тренировка</h1><p class="sub">${longDate(w.date)}</p>${stats(w)}${workoutCards(w)}`;
  },

  async progress() {
    const groups = await api('GET', 'groups');
    return `<h1>Прогресс</h1><p class="sub">Последние результаты и графики</p>${groupTiles(groups, 'progressGroup')}`;
  },

  async progressGroup({ id }) {
    const list = await api('GET', `groups/${id}/exercises`);
    const done = list.exercises.filter((ex) => ex.lastAt);
    const rows = done.map((ex) =>
      `<button class="row" data-go="progressExercise" data-id="${ex.id}">
         <span class="main"><div class="title">${esc(ex.name)}</div>
         <div class="desc">${shortDate(ex.lastAt)}: ${setsText(ex.lastSets)}</div></span>${chev}
       </button>`).join('');
    return `<h1>${esc(cap(list.group.name))}</h1>
      <p class="sub">Последний результат по каждому упражнению</p>
      ${rows ? `<div class="card">${rows}</div>` : '<div class="card empty">По этой группе пока нет записей.</div>'}
      <img class="chart" data-chart="groups/${id}/chart.png" alt="Графики макс. веса по упражнениям группы" hidden>`;
  },

  async progressExercise({ id }) {
    const p = await api('GET', `exercises/${id}/progress`);
    const rows = p.points.slice().reverse().slice(0, 12).map((pt) =>
      `<div class="row"><span class="main">${setsText(pt.sets)}</span><span class="aside">${shortDate(pt.date)}</span></div>`).join('');
    return `<h1>${esc(p.exercise.name)}</h1>
      <p class="sub">За последние полгода</p>
      ${p.hasChart ? `<img class="chart" data-chart="exercises/${id}/chart.png" alt="График макс. веса и объёма" hidden>` : ''}
      <div class="section">Тренировки</div>
      ${rows ? `<div class="card">${rows}</div>` : '<div class="card empty">Записей пока нет.</div>'}
      ${p.hasChart ? '' : '<p class="note">График появится после двух тренировок с этим упражнением.</p>'}`;
  },
};

function checkRow(kind, checked) {
  return `<label class="row check"><input type="checkbox" data-load="${kind}"${checked ? ' checked' : ''}>
    <span class="main">${kindNames[kind]}</span></label>`;
}

const percent = (before, now) => {
  if (!before) return '';
  const p = Math.round((now - before) / before * 100);
  return p > 0 ? `+${p}%` : p < 0 ? `−${-p}%` : '0%';
};

const daysAgo = (n) => (n <= 0 ? 'сегодня' : n === 1 ? 'вчера' : `${n} дн. назад`);
const duration = (min) => (min < 60 ? `${min} мин` : min % 60 ? `${Math.floor(min / 60)} ч ${min % 60} мин` : `${min / 60} ч`);

function recordText(r) {
  switch (r.kind) {
    case 'weight': return `вес ${num(r.value)} кг (было ${num(r.previous)})`;
    case 'volume': return `объём ${thousands(r.value)} кг (было ${thousands(r.previous)})`;
    case 'reps': return `${num(r.value)} повторов в подходе (было ${num(r.previous)})`;
    default: return `${num(r.value)} повторов за тренировку (было ${num(r.previous)})`;
  }
}

// What changed against the last time; exercises that stayed as they were are left out.
function changeText(c) {
  const parts = [];
  if (c.kind === 'weight') {
    if (c.before.maxWeight !== c.now.maxWeight) parts.push(`${num(c.before.maxWeight)} → ${num(c.now.maxWeight)} кг`);
    const p = c.before.volume !== c.now.volume ? percent(c.before.volume, c.now.volume) : '';
    if (p) parts.push('объём ' + p);
  } else {
    if (c.kind === 'band' && c.before.band !== c.now.band) parts.push(`резинка ${c.before.band} → ${c.now.band}`);
    if (c.before.totalReps !== c.now.totalReps) parts.push(`повторов ${c.before.totalReps} → ${c.now.totalReps}`);
  }
  return parts.length ? `${esc(c.exercise)}: ${parts.join(', ')}` : '';
}

// The summary of a finished workout under its numbers: what was better than ever,
// how it went against last time and how often the user trains.
function reportHtml(r) {
  let html = '';
  if (r.records.length) {
    html += `<div class="section">🏆 Рекорды</div><div class="card">${r.records.map((x) =>
      `<div class="row"><span class="main"><div class="title">${esc(x.exercise)}</div><div class="desc">${recordText(x)}</div></span></div>`).join('')}</div>`;
  }
  const changes = r.changes.map(changeText).filter(Boolean);
  if (changes.length) {
    html += `<div class="section">📈 Против прошлого раза</div><div class="card">${changes.map((t) =>
      `<div class="row"><span class="main">${t}</span></div>`).join('')}</div>`;
  }
  if (r.firstTime.length) html += `<p class="note">🆕 Впервые: ${esc(r.firstTime.join(', '))}</p>`;
  const freq = [];
  if (r.weekWorkouts > 1) freq.push(`тренировок за 7 дней: ${r.weekWorkouts}`);
  if (r.prevDate) freq.push(`прошлая — ${daysAgo(r.daysSincePrev)}`);
  if (freq.length) html += `<p class="note">📅 ${freq.join(' · ')}</p>`;
  return html;
}

function workoutCards(w) {
  const groups = new Map();
  for (const ex of w.exercises) {
    if (!groups.has(ex.group)) groups.set(ex.group, []);
    groups.get(ex.group).push(ex);
  }
  let html = '';
  for (const [name, list] of groups) {
    html += `<div class="section">${esc(name)}</div><div class="card">${list.map((ex) =>
      `<div class="row"><span class="main"><div class="title">${esc(ex.name)}</div>
       <div class="desc">${setsText(ex.sets)}</div></span></div>`).join('')}</div>`;
  }
  return html;
}

// The fields of a set for the kind of load: reps and the weight, the extra weight or the band level.
const setFields = {
  weight: { id: 'weight', label: 'Вес, кг', by: 2.5, min: 0, max: 1000, start: (n) => num(n.weight || 0) },
  bodyweight: { id: 'weight', label: 'Доп. вес, кг', by: 2.5, min: 0, max: 1000, start: (n) => num(n.weight || 0) },
  band: { id: 'band', label: `Резинка, уровень (1–${MAX_BAND})`, by: 1, min: 1, max: MAX_BAND, start: (n) => String(n.band || 5) },
};

// The screen of the exercise being logged: today's sets and a form prefilled
// with the most likely next set.
function exerciseView(log) {
  const id = log.exercise.id;
  const kind = log.kind || 'weight';
  const field = setFields[kind];
  const next = log.suggestion || { reps: 10 };
  const last = log.sets.length - 1;
  const today = log.sets.map((s, i) =>
    `<div class="row"><span class="num">${i + 1}</span><span class="main">${setLine(s)}</span>
     ${i === last ? `<button class="link danger" data-act="undo" data-id="${id}">Убрать</button>` : ''}</div>`).join('');
  const pills = log.exercise.loads.length > 1
    ? `<div class="pills" role="group" aria-label="Нагрузка">${log.exercise.loads.map((k) =>
      `<button type="button" class="pill${k === kind ? ' on' : ''}" aria-pressed="${k === kind}" data-act="set-kind" data-id="${id}" data-kind="${k}">${kindNames[k]}</button>`).join('')}</div>`
    : '';
  return `<h1>${esc(log.exercise.name)}</h1>
    <p class="sub">${log.lastAt ? `Прошлый раз, ${shortDate(log.lastAt)}: ${setsText(log.lastSets)}` : 'Раньше не делали'}</p>
    ${pills}
    ${today ? `<div class="section">Сегодня</div><div class="card">${today}</div>` : ''}
    <div class="section">${today ? 'Следующий подход' : 'Первый подход'}</div>
    <form class="card" data-form="set" data-id="${id}" data-kind="${kind}">
      <div class="stepper">
        <label for="reps">Повторы</label>
        <button type="button" data-step="reps" data-by="-1" data-min="1" data-max="1000" aria-label="Меньше повторов">−</button>
        <input id="reps" name="reps" inputmode="numeric" autocomplete="off" value="${next.reps}">
        <button type="button" data-step="reps" data-by="1" data-min="1" data-max="1000" aria-label="Больше повторов">+</button>
      </div>
      <div class="stepper">
        <label for="${field.id}">${field.label}</label>
        <button type="button" data-step="${field.id}" data-by="${-field.by}" data-min="${field.min}" data-max="${field.max}" aria-label="Меньше">−</button>
        <input id="${field.id}" name="${field.id}" inputmode="${kind === 'band' ? 'numeric' : 'decimal'}" autocomplete="off" value="${field.start(next)}">
        <button type="button" data-step="${field.id}" data-by="${field.by}" data-min="${field.min}" data-max="${field.max}" aria-label="Больше">+</button>
      </div>
    </form>
    <button class="btn" data-act="add-set" data-id="${id}">Записать подход</button>
    <button class="btn quiet" data-act="back">${today ? 'Готово' : 'К упражнениям'}</button>
    <button class="link" data-go="loads" data-id="${id}">Нагрузка упражнения</button>
    <img class="chart" data-chart="exercises/${id}/chart.png" alt="График упражнения" hidden>`;
}

function showExercise(log) {
  app.innerHTML = '<button class="back" data-act="back">‹ Назад</button>' + exerciseView(log);
  loadCharts();
}

const parseNumber = (text) => parseFloat(String(text).replace(',', '.'));

// readSet reads the form; null if the numbers do not make a set.
function readSet() {
  const kind = app.querySelector('form[data-form="set"]').dataset.kind;
  const reps = parseNumber(app.querySelector('#reps').value);
  if (!Number.isInteger(reps) || reps < 1 || reps > 1000) return null;
  if (kind === 'band') {
    const band = parseNumber(app.querySelector('#band').value);
    return Number.isInteger(band) && band >= 1 && band <= MAX_BAND ? { reps, band, kind } : null;
  }
  const weight = parseNumber(app.querySelector('#weight').value);
  if (!(weight >= 0) || weight > 1000) return null;
  return kind === 'bodyweight' ? { reps, weight, kind } : { reps, weight };
}

// ---------- actions

const actions = {
  back: () => history.back(),
  home: goHome,
  retry: render,

  async 'new-workout'(el) {
    await api('POST', 'workout', { force: Boolean(el.dataset.force) });
    go('workout');
    go('groups');
  },

  async finish() {
    const result = await api('POST', 'workout/finish');
    haptic('success');
    replace('finished', { result });
  },

  async 'add-set'(el) {
    const set = readSet();
    if (!set) { toast(`Повторы — целое число от 1; вес — от 0 кг; резинка — от 1 до ${MAX_BAND}.`); return; }
    showExercise(await api('POST', `exercises/${el.dataset.id}/sets`, set));
    haptic('light');
  },

  async 'set-kind'(el) {
    showExercise(await api('PUT', `exercises/${el.dataset.id}/kind`, { kind: el.dataset.kind }));
  },

  async undo(el) {
    showExercise(await api('DELETE', `exercises/${el.dataset.id}/sets/last`));
  },

  'more-history': (el) => replace('history', { pages: Number(el.dataset.pages) }),
};

function haptic(kind) {
  if (!tg || !tg.HapticFeedback) return;
  if (kind === 'success') tg.HapticFeedback.notificationOccurred('success');
  else tg.HapticFeedback.impactOccurred(kind);
}

// A button with data-confirm acts on the second press: the first one only changes its label.
function confirmed(el) {
  if (!el.dataset.confirm || el.dataset.armed) return true;
  const label = el.textContent;
  el.dataset.armed = '1';
  el.textContent = el.dataset.confirm;
  setTimeout(() => { delete el.dataset.armed; el.textContent = label; }, 4000);
  return false;
}

async function run(el, fn) {
  if (el.disabled) return;
  el.disabled = true;
  try {
    await fn(el);
  } catch (err) {
    if (err.code === 'no_active_workout') { toast('Тренировка уже завершена.'); goHome(); return; }
    toast(messageFor(err));
  } finally {
    el.disabled = false;
  }
}

app.addEventListener('click', (e) => {
  const step = e.target.closest('[data-step]');
  if (step) {
    const input = app.querySelector('#' + step.dataset.step);
    const value = (parseNumber(input.value) || 0) + Number(step.dataset.by);
    input.value = num(Math.min(Number(step.dataset.max), Math.max(Number(step.dataset.min), value)));
    return;
  }
  const link = e.target.closest('[data-go]');
  if (link) {
    go(link.dataset.go, link.dataset.id ? { id: Number(link.dataset.id) } : {});
    return;
  }
  const el = e.target.closest('[data-act]');
  if (el && actions[el.dataset.act] && confirmed(el)) run(el, actions[el.dataset.act]);
});

// Ticking a kind of load on the settings screen saves it at once; the first kind stays the default.
app.addEventListener('change', async (e) => {
  const box = e.target.closest('[data-load]');
  const panel = e.target.closest('[data-loads]');
  if (!box || !panel) return;
  const checked = kindOrder.filter((k) => panel.querySelector(`[data-load="${k}"]`).checked);
  if (!checked.length) {
    box.checked = true;
    toast('У упражнения должен остаться хотя бы один вид нагрузки.');
    return;
  }
  try {
    await api('PUT', `exercises/${panel.dataset.loads}/loads`, { loads: checked });
  } catch (err) {
    box.checked = !box.checked;
    toast(messageFor(err));
  }
});

app.addEventListener('submit', (e) => {
  e.preventDefault();
  const form = e.target;
  if (form.dataset.form === 'set') {
    // Enter on the keyboard records the set, same as the button.
    const button = app.querySelector('[data-act="add-set"]');
    run(button, actions['add-set']);
    return;
  }
  if (form.dataset.form === 'new-exercise') {
    const name = form.elements.name.value.trim();
    if (name.length < 2) { toast('Напишите название упражнения.'); return; }
    const loads = kindOrder.filter((k) => form.querySelector(`[data-load="${k}"]`).checked);
    if (!loads.length) { toast('Отметьте хотя бы один вид нагрузки.'); return; }
    run(form.querySelector('button'), async () => {
      const log = await api('POST', `groups/${form.dataset.id}/exercises`, { name, loads });
      go('exercise', { id: log.exercise.id });
    });
  }
});

// ---------- start

// A link may open a particular screen: ?screen=workout or ?screen=progressExercise&id=5.
const query = new URLSearchParams(location.search);
if (!history.state && screens[query.get('screen')]) {
  const id = Number(query.get('id'));
  history.replaceState({ screen: query.get('screen'), params: id ? { id } : {}, depth: 0 }, '');
}
render();

// The Telegram script only adds comfort (native back button, haptics, theme updates),
// so it is loaded after the page is ready and the app works even if it never arrives.
window.addEventListener('load', () => {
  if (!launch.has('tgWebAppData')) return;
  const script = document.createElement('script');
  script.src = 'https://telegram.org/js/telegram-web-app.js';
  script.onload = () => {
    tg = window.Telegram && window.Telegram.WebApp;
    if (!tg) return;
    tg.ready();
    tg.expand();
    tg.BackButton.onClick(() => history.back());
    document.documentElement.classList.add('in-telegram');
    if (current().depth > 0) tg.BackButton.show();
  };
  document.head.appendChild(script);
});
