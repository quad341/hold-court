(function () {
	"use strict";

	var holds = JSON.parse(document.getElementById("holds-data").textContent);
	var folders = JSON.parse(document.getElementById("folders-data").textContent);
	var recordOnly = JSON.parse(document.getElementById("mode-data").textContent).record_only;
	var consumerDescription = document.getElementById('consumer-description').textContent;
	var byID = {};
	holds.forEach(function (h) { byID[h.id] = h; });

	var state = {
		folder: (folders[0] && folders[0].id) || "inbox",
		cursor: 0,
		focus: "list", // "list" | "reading"
		pending: {}, // holdID -> {action, note}
		drafts: {}, // holdID -> note text typed via 'i' before an action is chosen
		filterQuery: "",
		matches: [],
		matchCursor: -1,
		pendingG: false,
		updates: {},
		retained: null,
		saving: false,
		etag: "",
	};

	var listEl = document.getElementById("pane-list");
	var foldersEl = document.getElementById("pane-folders");
	var readingEl = document.getElementById("pane-reading");
	var pendingBarEl = document.getElementById("pending-bar");
	var cheatsheetEl = document.getElementById("cheatsheet-overlay");
	var noticeEl = document.getElementById("notice");
	var liveEl = document.getElementById("live-status");
	var activityEl = document.getElementById("activity-button");
	var displayedRevision = "";
	var displayedActivityRevision = "";
	var readingTab = "review";

	function notice(message) {
		noticeEl.textContent = message;
		noticeEl.hidden = !message;
	}

	function persistDrafts() {
		try {
			localStorage.setItem("hold-court-drafts-v1", JSON.stringify({ pending: state.pending, drafts: state.drafts }));
		} catch (_) { notice("Draft backup unavailable in this browser. Keep this tab open until saved."); }
	}
	try {
		var backup = JSON.parse(localStorage.getItem("hold-court-drafts-v1") || "null");
		if (backup) { state.pending = backup.pending || {}; state.drafts = backup.drafts || {}; }
	} catch (_) { notice("Could not restore saved drafts."); }
	holds.forEach(function (h) { if (h.updated) state.updates[h.id] = true; });

	function visibleHolds() {
		var list = holds.filter(function (h) {
			if (h.id === state.retained) return true;
			if (state.folder === "updates") return !!state.updates[h.id];
			if (state.folder.indexOf("class:") === 0) {
				return h.class === state.folder.slice(6);
			}
			return h.state === state.folder;
		});
		if (state.filterQuery) {
			var q = state.filterQuery.toLowerCase();
			list = list.filter(function (h) {
				return (
					h.title.toLowerCase().indexOf(q) !== -1 ||
					h.question.toLowerCase().indexOf(q) !== -1 ||
					h.class.toLowerCase().indexOf(q) !== -1 ||
					h.repo.toLowerCase().indexOf(q) !== -1
				);
			});
		}
		return list;
	}

	function currentHold() {
		var list = visibleHolds();
		return list[state.cursor] || null;
	}

	function escapeHTML(s) {
		var div = document.createElement("div");
		div.textContent = s;
		return div.innerHTML;
	}

	function renderFolders() {
		var html = "";
		folders.forEach(function (f) {
			if (f.heading) {
				html += '<li class="folder-heading">' + escapeHTML(f.label) + "</li>";
				return;
			}
			var cls = f.id === state.folder ? "selected" : "";
			html +=
				'<li class="' + cls + '" data-folder-id="' + escapeHTML(f.id) + '">' +
				escapeHTML(f.label) +
				' <span class="hold-count">' + f.count + "</span></li>";
		});
		foldersEl.innerHTML = "<ul>" + html + "</ul>";
	}

	function renderList() {
		var list = visibleHolds();
		if (state.cursor >= list.length) state.cursor = Math.max(0, list.length - 1);

		var html = list
			.map(function (h, i) {
				var classes = [];
				if (h.unread) classes.push("unread");
				if (i === state.cursor) classes.push("selected");
				if (state.matches.indexOf(i) !== -1) classes.push("match");
				var dot = state.pending[h.id] ? '<span class="pending-dot">&bull;</span>' : "";
				var age = h.held_at ? h.held_at.slice(0, 10) : "";
				return (
					'<li class="' + classes.join(" ") + '" data-hold-id="' + escapeHTML(h.id) + '">' +
					dot + '<span class="hold-title">' + escapeHTML(h.title) + '</span>' +
					'<span class="hold-meta">' + escapeHTML(h.repo) + ' #' + h.pr + ' · ' + escapeHTML(age) +
					(state.updates[h.id] ? ' · <span class="activity-tag">' + (h.result && h.result.status === 'reply_ready' ? 'Reply ready' : 'Updated') + '</span>' : '') + '</span></li>'
				);
			})
			.join("");
		listEl.innerHTML = "<ul>" + html + "</ul>";
	}

	function rulingButtons(hold) {
		var actions = [
			["proceed", "p"],
			["changes", "c"],
			["close", "x"],
			["discuss", "d"],
		];
		var pending = state.pending[hold.id];
		return actions
			.map(function (pair) {
				var action = pair[0], key = pair[1];
				var active = pending && pending.action === action ? "active" : "";
				return (
					'<button type="button" class="' + active + '" data-action="' + action + '">' +
					key + ": " + ({proceed:"Accept recommendation",changes:"Request author changes",close:"Close PR",discuss:"Discuss"}[action]) + "</button>"
				);
			})
			.join(" ");
	}

	function renderReading() {
		var hold = currentHold();
		if (!hold) {
			readingEl.innerHTML = "<p>No holds in this folder.</p>";
			return;
		}
		readingEl.dataset.holdId = hold.id;
		displayedRevision = hold.revision;
		displayedActivityRevision = hold.activity_revision;
		var pending = state.pending[hold.id];
		var note = (pending && pending.note) || state.drafts[hold.id] || "";
		readingEl.innerHTML =
			'<div id="reading-content"><button id="show-update" type="button" hidden>New activity on this hold — show update</button>' +
			'<h1>' + escapeHTML(hold.title) + "</h1>" +
			'<p class="question">' + escapeHTML(hold.question) + "</p>" +
			'<p><a href="' + escapeHTML(hold.url) + '" target="_blank" rel="noopener">' +
			escapeHTML(hold.repo) + " #" + hold.pr + "</a> &middot; " + escapeHTML(hold.state) + "</p>" +
			'<div id="latest-status">' + latestStatus(hold) + '</div>' +
			'<div class="history-tabs"><button data-tab="review">Review</button><button data-tab="history">History &amp; discussion</button></div>' +
			'<div id="reading-tab">' +
			'<div class="review-body">' + hold.review_html + "</div></div></div>" +
			 '<div id="ruling-bar">' +
			'<p class="execution-mode">' + (recordOnly ? 'Record-only: saving does not send anything to MPR, an agent, or GitHub.' : escapeHTML(consumerDescription || 'Consumer configured: saving invokes the configured hook. Its policy determines external actions.')) + '</p>' +
			actionHelp() + rulingButtons(hold) +
			'<button type="button" id="clear-ruling">Clear choice</button>' +
			'<div><textarea id="note-input" rows="2" placeholder="annotate (i)">' +
			escapeHTML(note) +
			"</textarea></div>" +
			'<button type="button" id="save-btn">s: save pending rulings</button>' +
			"</div>";
		if (readingTab === "history") showHistory();
	}

	function latestStatus(hold) {
		if (hold.result) return '<strong>' + escapeHTML(hold.result.status.replace(/_/g, ' ')) + '</strong> — ' + escapeHTML(hold.result.summary || '') + ' <button data-tab="history">View history / replies</button>';
		if (hold.ruling) return 'Saved: ' + escapeHTML(hold.ruling.action) + (recordOnly ? ' (local record; not dispatched)' : ' (handoff pending)');
		return '';
	}

	function showHistory() {
		var id = readingEl.dataset.holdId;
		var target = document.getElementById('reading-tab');
		if (!target) return;
		target.innerHTML = '<p>Loading history…</p>';
		fetch('/api/holds/' + encodeURIComponent(id) + '/history', {cache:'no-store'})
			.then(function (response) { if (!response.ok) throw new Error('Could not load history'); return response.json(); })
			.then(function (entries) {
				if (readingEl.dataset.holdId !== id || readingTab !== 'history') return;
				var messages = {};
				entries.forEach(function (entry) {
					var data = entry.data;
					if (entry.kind === 'decision') messages['decision-' + entry.id] = {at:data.ruled_at, author:data.ruled_by + ' · ' + data.action, body:data.note || '(no note)'};
					if (entry.kind === 'discussion') data.forEach(function (m) { messages[m.id] = m; });
					if (entry.kind === 'result') (data.thread || []).forEach(function (m) { messages[m.id] = m; });
				});
				var conversation = Object.values(messages).sort(function (a,b) { return a.at.localeCompare(b.at); }).map(function (m) {
					return '<article class="history-entry"><strong>' + escapeHTML(m.author) + '</strong> · ' + escapeHTML(m.at) + '<pre>' + escapeHTML(m.body) + '</pre></article>';
				}).join('');
				var versions = entries.map(function (entry) {
					var data = entry.data;
					var label = entry.kind === 'review' ? 'Review version' : entry.kind === 'decision' ? 'Decision: ' + data.action : entry.kind === 'discussion' ? 'Conversation updated' : 'Consumer: ' + data.status;
					var body = entry.kind === 'review' ? (data.question + '\n\n' + data.review_body_md) : entry.kind === 'decision' ? data.note : entry.kind === 'discussion' ? data.map(function (m) { return m.author + ': ' + m.body; }).join('\n\n') : data.summary;
					return '<details class="history-entry"><summary>' + escapeHTML(entry.at + ' · ' + label) + '</summary><pre>' + escapeHTML(body || '') + '</pre></details>';
				}).join('');
				target.innerHTML = '<h2>Conversation</h2>' + (conversation || '<p>No messages yet.</p>') + '<h2>Observed history</h2><p>Newest first. Expand a version to read it. History starts when this server first observes the hold.</p>' + versions;
			}).catch(function (error) { if (readingEl.dataset.holdId === id && readingTab === 'history') target.textContent = error.message; });
	}

	function actionHelp() {
		return '<details class="action-help"><summary>What do these decisions mean?</summary><dl>' +
			'<dt>Accept recommendation (proceed)</dt><dd>Record agreement with the prepared recommendation. This does not itself mean “merge” or “publish”; the consumer must define that policy.</dd>' +
			'<dt>Request author changes</dt><dd>Record a request for PR changes. Revising our prepared review is a different workflow and is not implemented yet.</dd>' +
			'<dt>Close</dt><dd>Record a close decision and your rationale. No closing message is generated.</dd>' +
			'<dt>Discuss</dt><dd>Ask the configured consumer to investigate your note and reply in History &amp; discussion. In record-only mode, the question stays local.</dd>' +
			'</dl></details>';
	}

	function renderPendingBar() {
		var n = Object.keys(state.pending).length;
		if (n === 0) {
			pendingBarEl.hidden = true;
			return;
		}
		pendingBarEl.hidden = false;
		pendingBarEl.textContent = n + " pending ruling" + (n === 1 ? "" : "s") + " — press s to save";
	}

	function renderAll() {
		renderFolders();
		renderList();
		renderReading();
		renderPendingBar();
	}

	function setFolder(id) {
		saveNoteDraft();
		state.retained = null;
		state.folder = id;
		state.cursor = 0;
		state.filterQuery = "";
		state.matches = [];
		state.matchCursor = -1;
		renderAll();
	}

	function cycleFolder(delta) {
		var selectable = folders.filter(function (f) { return !f.heading; });
		var idx = selectable.findIndex(function (f) { return f.id === state.folder; });
		idx = (idx + delta + selectable.length) % selectable.length;
		setFolder(selectable[idx].id);
	}

	function markRead(holdID, unread) {
		var hold = byID[holdID];
		if (!hold) return;
		var revision = displayedActivityRevision;
		fetch("/api/holds/" + encodeURIComponent(holdID) + "/read", {
			method: "POST", headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ unread: unread, revision: revision }),
		}).then(function (resp) {
			if (!resp.ok) throw new Error("Read state could not be saved");
			hold.unread = unread;
			if (!unread && hold.activity_revision === revision) {
				delete state.updates[holdID];
				hold.updated = false;
			}
			updateActivity(); renderList();
		}).catch(function (err) { notice(err.message); });
	}

	function openSelected() {
		state.focus = "reading";
		var hold = currentHold();
		renderReading();
		if (hold) markRead(hold.id, false);
	}

	function setPendingAction(action) {
		var hold = currentHold();
		if (!hold) return;
		if (hold.state === "stood-down") { notice("This hold is resolved. Open a current hold before recording a decision."); return; }
		if (hold.revision !== displayedRevision) { notice("This hold changed. Show its update before choosing a decision."); return; }
		saveNoteDraft();
		var note = state.drafts[hold.id] || (state.pending[hold.id] && state.pending[hold.id].note) || "";
		if (state.pending[hold.id] && state.pending[hold.id].action === action) delete state.pending[hold.id];
		else state.pending[hold.id] = { action: action, note: note, revision: hold.revision };
		persistDrafts();
		renderList();
		renderReading();
		renderPendingBar();
	}

	function clearPending() {
		saveNoteDraft();
		var hold = currentHold();
		if (!hold) return;
		delete state.pending[hold.id];
		persistDrafts(); renderList(); renderReading(); renderPendingBar();
		notice('Choice cleared. Your note is kept; nothing will be sent for this hold.');
	}

	function saveNoteDraft() {
		var id = readingEl.dataset.holdId;
		var input = document.getElementById("note-input");
		if (!id || !input) return;
		state.drafts[id] = input.value;
		if (state.pending[id]) state.pending[id].note = input.value;
		persistDrafts();
	}

	function focusNoteInput() {
		var input = document.getElementById("note-input");
		if (input) input.focus();
	}

	function savePendingRulings() {
		if (state.saving) return;
		saveNoteDraft();
		var submitted = JSON.parse(JSON.stringify(state.pending));
		var items = Object.keys(submitted).map(function (id) {
			return { hold_id: id, action: submitted[id].action, note: submitted[id].note || "", revision: submitted[id].revision || "" };
		});
		if (!items.length) return;
		if (!recordOnly) {
			if (items.some(function (item) { return item.action !== 'proceed' && !item.note.trim(); })) {
				notice('Add a note for discuss, author changes, or closing. The note is the exact message/instruction sent.'); return;
			}
			var preview = items.map(function (item) {
				var hold = byID[item.hold_id];
				return item.action.toUpperCase() + ': ' + hold.repo + ' #' + hold.pr + '\nHead: ' + hold.head_sha + '\nMPR recommendation: ' + (hold.verdict || 'unspecified') + '\n' + item.note;
			}).join('\n\n');
			if (!window.confirm((consumerDescription || 'Send to the configured consumer?') + '\n\n' + preview + '\n\nSend these decisions?')) return;
		}
		state.saving = true;
		fetch("/api/rulings", {
			method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(items),
		}).then(function (resp) {
			if (!resp.ok) throw new Error("Save failed (HTTP " + resp.status + "). Your drafts are retained.");
			return resp.json();
		}).then(function (results) {
			var errors = [];
			results.forEach(function (r) {
				if (!r.ok) { errors.push(r.error); return; }
				if (r.hold) {
					byID[r.hold_id] = r.hold;
					holds = holds.map(function (h) { return h.id === r.hold_id ? r.hold : h; });
					if (readingEl.dataset.holdId === r.hold_id) {
						state.retained = r.hold_id;
						displayedRevision = r.hold.revision;
						var status = document.getElementById('latest-status');
						if (status) status.innerHTML = latestStatus(r.hold);
						if (readingTab === 'history') showHistory();
					}
				}
				if (JSON.stringify(state.pending[r.hold_id]) === JSON.stringify(submitted[r.hold_id])) {
					delete state.pending[r.hold_id];
					delete state.drafts[r.hold_id];
					if (readingEl.dataset.holdId === r.hold_id) {
						var input = document.getElementById('note-input');
						if (input && input.value === submitted[r.hold_id].note) input.value = '';
					}
				}
			});
			persistDrafts(); renderPendingBar();
			notice(errors.length ? errors.join("; ") : "Decisions saved. See History & discussion for the record and any replies.");
		}).catch(function (err) { notice(err.message); }).finally(function () {
			state.saving = false;
			pollHolds();
		});
	}

	function updateActivity() {
		activityEl.textContent = 'Updates (' + Object.keys(state.updates).length + ')';
	}

	var polling = false;
	function pollHolds() {
		if (polling || state.saving) return;
		polling = true;
		fetch('/api/holds', { headers: state.etag ? {'If-None-Match': state.etag} : {}, cache: 'no-store' })
			.then(function (resp) {
				if (resp.status === 304) return null;
				if (!resp.ok) throw new Error('Live updates unavailable; retrying automatically');
				state.etag = resp.headers.get('ETag') || '';
				return resp.json();
			}).then(function (data) {
				liveEl.textContent = 'Live · checked ' + new Date().toLocaleTimeString();
				if (!data) return;
				var selected = currentHold();
				var selectedID = selected && selected.id;
				data.holds.forEach(function (h) {
					var old = byID[h.id];
					if (h.updated || (old && old.activity_revision !== h.activity_revision) || (!old && h.state === 'inbox')) state.updates[h.id] = true;
				});
				holds = data.holds;
				byID = {};
				holds.forEach(function (h) { byID[h.id] = h; });
				// Retain the current item even if its folder changes. Reading context
				// and the textarea DOM stay untouched until the user opens an update.
				if (selected && !byID[selectedID]) {
					selected = Object.assign({}, selected, {state:'stood-down', resolved_reason:'Removed from the current feed', revision:selected.revision.replace(/-removed$/, '') + '-removed', activity_revision:selected.activity_revision.replace(/-removed$/, '') + '-removed'});
					state.updates[selectedID] = true;
					 holds.push(selected); byID[selectedID] = selected;
				}
				state.retained = selectedID;
				folders = data.folders;
				var index = visibleHolds().findIndex(function (h) { return h.id === selectedID; });
				state.cursor = Math.max(0, index);
				renderFolders(); renderList(); updateActivity();
				var button = document.getElementById('show-update');
				if (button && byID[selectedID]) button.hidden = byID[selectedID].activity_revision === displayedActivityRevision;
				if (!selectedID && currentHold()) renderReading();
			}).catch(function (err) { liveEl.textContent = err.message; }).finally(function () { polling = false; });
	}

	function applyFilter(query) {
		state.filterQuery = query;
		state.cursor = 0;
		var list = visibleHolds();
		state.matches = list.map(function (_, i) { return i; });
		state.matchCursor = state.matches.length ? 0 : -1;
		if (state.matchCursor !== -1) state.cursor = state.matches[state.matchCursor];
		renderList();
		renderReading();
	}

	function nextMatch(delta) {
		if (!state.matches.length) return;
		state.matchCursor = (state.matchCursor + delta + state.matches.length) % state.matches.length;
		state.cursor = state.matches[state.matchCursor];
		renderList();
		renderReading();
	}

	function openFilterPrompt() {
		var q = window.prompt("filter/search holds:", state.filterQuery);
		if (q === null) return;
		applyFilter(q);
	}

	function scrollReading(dir) {
		var content = document.getElementById('reading-content');
		if (content) content.scrollBy(0, dir * (content.clientHeight / 2));
	}

	// --- event wiring -----------------------------------------------------

	foldersEl.addEventListener("click", function (ev) {
		var li = ev.target.closest("li[data-folder-id]");
		if (li) setFolder(li.getAttribute("data-folder-id"));
	});

	listEl.addEventListener("click", function (ev) {
		var li = ev.target.closest("li[data-hold-id]");
		if (!li) return;
		var list = visibleHolds();
		var idx = list.findIndex(function (h) { return h.id === li.getAttribute("data-hold-id"); });
		if (idx !== -1) {
			state.cursor = idx;
			openSelected();
			renderList();
		}
	});

	readingEl.addEventListener("input", function (ev) {
		if (ev.target.id === "note-input") saveNoteDraft();
	});
	readingEl.addEventListener("click", function (ev) {
		var tabButton = ev.target.closest('[data-tab]');
		if (tabButton) { saveNoteDraft(); readingTab = tabButton.dataset.tab; renderReading(); return; }
		if (ev.target.id === 'clear-ruling') { clearPending(); return; }
		if (ev.target.id === "show-update") { saveNoteDraft(); openSelected(); return; }
		var btn = ev.target.closest("button[data-action]");
		if (btn) {
			setPendingAction(btn.getAttribute("data-action"));
			return;
		}
		if (ev.target.id === "save-btn") savePendingRulings();
	});

	readingEl.addEventListener("focusin", function (ev) {
		if (ev.target.id === "note-input") state.focus = "note";
	});

	readingEl.addEventListener("focusout", function (ev) {
		if (ev.target.id === "note-input") {
			saveNoteDraft();
			state.focus = "reading";
		}
	});

	document.addEventListener("keydown", function (ev) {
		var typingInField =
			document.activeElement &&
			(document.activeElement.tagName === "TEXTAREA" || document.activeElement.tagName === "INPUT");

		if (typingInField) {
			if (ev.key === "Escape") {
				document.activeElement.blur();
				ev.preventDefault();
			}
			return;
		}

		if (ev.key === "Escape") {
			if (!cheatsheetEl.hidden) cheatsheetEl.hidden = true;
			else clearPending();
			return;
		}

		if (ev.ctrlKey && ev.key === "d") {
			scrollReading(1);
			ev.preventDefault();
			return;
		}
		if (ev.ctrlKey && ev.key === "u") {
			scrollReading(-1);
			ev.preventDefault();
			return;
		}

		var wasPendingG = state.pendingG;
		state.pendingG = false;

		switch (ev.key) {
			case "j":
				state.cursor = Math.min(visibleHolds().length - 1, state.cursor + 1);
				renderList();
				renderReading();
				break;
			case "k":
				state.cursor = Math.max(0, state.cursor - 1);
				renderList();
				renderReading();
				break;
			case "g":
				if (wasPendingG) {
					state.cursor = 0;
					renderList();
					renderReading();
				} else {
					state.pendingG = true;
				}
				break;
			case "G":
				state.cursor = Math.max(0, visibleHolds().length - 1);
				renderList();
				renderReading();
				break;
			case "Enter":
			case "l":
				openSelected();
				renderList();
				break;
			case "h":
				state.focus = "list";
				break;
			case "Tab":
				cycleFolder(ev.shiftKey ? -1 : 1);
				ev.preventDefault();
				break;
			case "/":
				openFilterPrompt();
				ev.preventDefault();
				break;
			case "n":
				nextMatch(1);
				break;
			case "N":
				nextMatch(-1);
				break;
			case "p":
				setPendingAction("proceed");
				break;
			case "c":
				setPendingAction("changes");
				break;
			case "x":
				setPendingAction("close");
				break;
			case "d":
				setPendingAction("discuss");
				break;
			case "i":
				openSelected();
				focusNoteInput();
				ev.preventDefault();
				break;
			case "u":
				var hold = currentHold();
				if (hold) markRead(hold.id, !hold.unread);
				break;
			case "o":
				var h2 = currentHold();
				if (h2) window.open(h2.url, "_blank", "noopener");
				break;
			case "s":
				savePendingRulings();
				break;
			case "?":
				cheatsheetEl.hidden = !cheatsheetEl.hidden;
				break;
		}
		if (["j", "k", "g", "G", "n", "N", "Enter", "l"].indexOf(ev.key) !== -1) {
			var selectedRow = listEl.querySelector('li.selected');
			if (selectedRow) selectedRow.scrollIntoView({block: 'nearest'});
		}
	});

	cheatsheetEl.addEventListener("click", function (ev) {
		if (ev.target === cheatsheetEl) cheatsheetEl.hidden = true;
	});

	activityEl.addEventListener('click', function () { setFolder('updates'); });
	window.addEventListener('beforeunload', function (ev) {
		if (Object.keys(state.pending).length || Object.values(state.drafts).some(Boolean)) {
			ev.preventDefault(); ev.returnValue = '';
		}
	});
	renderAll(); updateActivity(); pollHolds();
	setInterval(pollHolds, 5000);
})();
