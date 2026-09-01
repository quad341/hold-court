(function () {
	"use strict";

	var holds = JSON.parse(document.getElementById("holds-data").textContent);
	var folders = JSON.parse(document.getElementById("folders-data").textContent);
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
	};

	var listEl = document.getElementById("pane-list");
	var foldersEl = document.getElementById("pane-folders");
	var readingEl = document.getElementById("pane-reading");
	var pendingBarEl = document.getElementById("pending-bar");
	var cheatsheetEl = document.getElementById("cheatsheet-overlay");

	function visibleHolds() {
		var list = holds.filter(function (h) {
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
					dot + escapeHTML(h.title) +
					' <span class="hold-count">' + escapeHTML(age) + "</span></li>"
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
					key + ": " + action + "</button>"
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
		var pending = state.pending[hold.id];
		var note = (pending && pending.note) || state.drafts[hold.id] || "";
		readingEl.innerHTML =
			'<h1>' + escapeHTML(hold.title) + "</h1>" +
			'<p class="question">' + escapeHTML(hold.question) + "</p>" +
			'<p><a href="' + escapeHTML(hold.url) + '" target="_blank" rel="noopener">' +
			escapeHTML(hold.repo) + " #" + hold.pr + "</a> &middot; " + escapeHTML(hold.state) + "</p>" +
			'<div class="review-body">' + hold.review_html + "</div>" +
			'<div id="ruling-bar">' +
			rulingButtons(hold) +
			'<div><textarea id="note-input" rows="2" placeholder="annotate (i)">' +
			escapeHTML(note) +
			"</textarea></div>" +
			'<button type="button" id="save-btn">s: save pending rulings</button>' +
			"</div>";
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
		if (!hold || hold.unread === unread) return;
		hold.unread = unread;
		fetch("/api/holds/" + encodeURIComponent(holdID) + "/read", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ unread: unread }),
		}).catch(function () {});
		renderList();
	}

	function openSelected() {
		state.focus = "reading";
		var hold = currentHold();
		if (hold && hold.unread) markRead(hold.id, false);
		renderReading();
	}

	function setPendingAction(action) {
		var hold = currentHold();
		if (!hold) return;
		var note = state.drafts[hold.id] || (state.pending[hold.id] && state.pending[hold.id].note) || "";
		state.pending[hold.id] = { action: action, note: note };
		renderList();
		renderReading();
		renderPendingBar();
	}

	function saveNoteDraft() {
		var hold = currentHold();
		var input = document.getElementById("note-input");
		if (!hold || !input) return;
		var note = input.value;
		state.drafts[hold.id] = note;
		if (state.pending[hold.id]) state.pending[hold.id].note = note;
	}

	function focusNoteInput() {
		var input = document.getElementById("note-input");
		if (input) input.focus();
	}

	function savePendingRulings() {
		saveNoteDraft();
		var items = Object.keys(state.pending).map(function (id) {
			return {
				hold_id: id,
				action: state.pending[id].action,
				note: state.pending[id].note || "",
			};
		});
		if (items.length === 0) return;

		fetch("/api/rulings", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(items),
		})
			.then(function (resp) { return resp.json(); })
			.then(function (results) {
				(results || []).forEach(function (r) {
					if (!r.ok) return;
					delete state.pending[r.hold_id];
					delete state.drafts[r.hold_id];
					var hold = byID[r.hold_id];
					if (hold) hold.state = "ruled";
				});
				renderAll();
			})
			.catch(function () {});
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
		readingEl.scrollBy(0, dir * (readingEl.clientHeight / 2));
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

	readingEl.addEventListener("click", function (ev) {
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
	});

	cheatsheetEl.addEventListener("click", function (ev) {
		if (ev.target === cheatsheetEl) cheatsheetEl.hidden = true;
	});

	renderAll();
})();
