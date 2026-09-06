"""Browser regression checks. Requires Playwright and its Chromium browser.
Run via make test-browser; all feed, database, and ruling data is temporary.
"""
import json
from datetime import datetime, timezone
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import sys
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]


def write_json(path, data):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(data))
    temporary.replace(path)


with tempfile.TemporaryDirectory(prefix='hold-court-live-test-') as tmp:
    tmp = Path(tmp)
    feed = tmp / 'feed'
    rulings = tmp / 'rulings'
    feed.mkdir()
    rulings.mkdir()
    title = 'A long actual pull request title explaining a subtle process-group cancellation race across foreground children'
    hold = dict(id='example-42-head', repo='example/widgets', pr=42, author='Contributor',
                decision_context_md='## Decision requiring your response\nReviewers disagree on whether to fix shutdown reporting before merging.\n\n### Claude — fix-merge\nEmit a warning when cleanup skips a child, and test that warning.\n\n### Codex — auto-merge\nCleanup fails safely; reporting can follow in a separate change.\n\n### Contract change\nBefore: skipped cleanup reports success. After: report skipped work explicitly.',
                title='example/widgets #42: ' + title, question='Should this behavior change?',
                review_body_md='## Prepared review\n\nThe cancellation change preserves the process-group boundary during shutdown. The remaining decision is whether cleanup should wait for child processes to finish.\n\n' + ('### Verification\n\nExercise cancellation with a foreground child and confirm that cleanup leaves no orphan process.\n\n' * 20),
                head_sha='a' * 40, held_at='2026-09-01T12:00:00Z',
                url='https://github.com/example/widgets/pull/42', **{'class': 'scope'})
    write_json(feed / 'one.json', hold)
    consumer_config = dict(repos=['example/widgets'], rulings=str(rulings), spool=str(tmp/'requests'), target='test-agent')
    write_json(tmp/'consumer.json', consumer_config)
    hook = [sys.executable, str(ROOT/'adapters/mpr/consumer.py'), 'enqueue', '--config', str(tmp/'consumer.json')]
    (tmp/'holdcourt.toml').write_text('on_ruling = ' + json.dumps(hook) + '\nconsumer_description = "Send to the isolated test queue"\n')
    server = subprocess.Popen([str(ROOT / 'hold-court'), 'serve', '-feed', str(feed),
                               '-rulings', str(rulings), '-db', str(tmp / 'state.db')],
                              cwd=tmp, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                              text=True, start_new_session=True)
    try:
        url = server.stdout.readline().strip().split('session: ', 1)[1]
        with sync_playwright() as pw:
            browser = pw.chromium.launch(executable_path=os.environ.get('HOLD_COURT_CHROMIUM'))
            page = browser.new_page(viewport={'width': 1280, 'height': 900}, color_scheme='dark')
            def doc_screenshot(name):
                directory = os.environ.get('HOLD_COURT_DOC_SCREENSHOTS')
                if not directory:
                    return
                Path(directory).mkdir(parents=True, exist_ok=True)
                page.set_viewport_size({'width':1280, 'height':1100})
                page.screenshot(path=str(Path(directory)/name))
                page.set_viewport_size({'width':1280, 'height':900})

            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            confirmations = []
            page.on('dialog', lambda dialog: (confirmations.append(dialog.message), dialog.accept()))
            page.goto(url)
            expect(page.locator('.hold-title')).to_have_text(title)
            expect(page.locator('.decision-context')).to_contain_text('Emit a warning when cleanup skips a child')
            expect(page.locator('.decision-context')).to_contain_text('Before: skipped cleanup reports success')
            expect(page.locator('.hold-meta')).to_contain_text('@Contributor')
            expect(page.locator('#reading-content .hold-author')).to_have_text('Author: @Contributor')
            # The page is a shell: holds arrive from /api/holds and are kept in
            # IndexedDB, so a reload renders from that copy even while the
            # server is unreachable, then revalidates by ETag.
            expect(page.locator('#data-status')).to_contain_text('fetched')
            version_before = page.locator('#data-status').inner_text().split()[1]
            assert len(version_before) == 16, version_before
            assert 'holds-data' not in page.content()
            page.route('**/api/holds', lambda route: route.abort())
            page.reload()
            expect(page.locator('.hold-title')).to_have_text(title)
            expect(page.locator('#data-status')).to_contain_text('cached, checking')
            expect(page.locator('#cache-notice')).to_be_hidden()
            page.unroute('**/api/holds')
            expect(page.locator('#data-status')).to_contain_text('verified', timeout=15000)
            expect(page.locator('#data-status')).to_contain_text(version_before)
            # Rebuild cache: drops the local copy, rescans on the server, and
            # always reports what happened, even when nothing changed.
            page.locator('#rebuild-cache').click()
            expect(page.locator('#cache-notice')).to_contain_text('Data cache rebuilt: version ' + version_before + ' (unchanged)')
            expect(page.locator('#cache-notice')).to_contain_text('manual rebuild')
            expect(page.locator('.hold-title')).to_have_text(title)
            page.locator('#cache-notice-dismiss').click()
            expect(page.locator('#cache-notice')).to_be_hidden()
            page.locator('#note-input').fill('Keep this draft across search')
            page.keyboard.press('Escape')
            page.keyboard.press('/')
            expect(page.locator('#search-input')).to_be_focused()
            page.locator('#search-input').fill('author:CONTRIBUTOR')
            expect(page.locator('#pane-list li')).to_have_count(1)
            expect(page.locator('#search-summary')).to_contain_text('1 of 1 holds')
            expect(page.locator('#search-summary')).to_contain_text('author: matches an exact login')
            page.locator('#search-input').fill('author:contrib')
            expect(page.locator('#pane-list li')).to_have_count(0)
            expect(page.locator('#search-summary')).to_contain_text('0 of 1 holds')
            page.locator('#clear-search').click()
            page.locator('#search-field').select_option('author')
            page.locator('#search-input').fill('contrib')
            expect(page.locator('#pane-list li')).to_have_count(1)
            expect(page.locator('#note-input')).to_have_value('Keep this draft across search')
            page.locator('#search-input').fill('widgets')
            expect(page.locator('#pane-list li')).to_have_count(0)
            page.locator('#search-field').select_option('summary')
            expect(page.locator('#pane-list li')).to_have_count(1)
            expect(page.locator('#search-summary')).to_contain_text('review and history excluded')
            page.locator('#clear-search').click()
            for query, count in [('#42', 1), ('pr:42', 1), ('pr:#42', 1), ('#4', 0), ('pr:420', 0), ('author:Contributor pr:42 widgets', 1), ('pr:43', 0)]:
                page.locator('#search-input').fill(query)
                expect(page.locator('#pane-list li')).to_have_count(count)
                expect(page.locator('#search-summary')).to_contain_text('exact PR number')
            page.locator('#search-field').select_option('pr')
            for query, count in [('42', 1), ('4', 0), ('widgets', 0), ('#42', 1), ('pr:42', 1)]:
                page.locator('#search-input').fill(query)
                expect(page.locator('#pane-list li')).to_have_count(count)
            expect(page.locator('#note-input')).to_have_value('Keep this draft across search')
            page.locator('#search-field').select_option('summary')
            page.locator('#clear-search').click()
            page.locator('#search-input').press('Escape')
            list_box = page.locator('#pane-list').bounding_box()
            read_box = page.locator('#pane-reading').bounding_box()
            assert list_box['width'] > 900 and read_box['y'] >= list_box['y'] + list_box['height']
            assert page.locator('.hold-title').evaluate('(el) => getComputedStyle(el.parentElement).whiteSpace') == 'normal'
            divider = page.locator('#reading-divider').bounding_box()
            page.mouse.move(divider['x']+30, divider['y']+5)
            page.mouse.down()
            page.mouse.move(divider['x']+30, divider['y']+55)
            page.mouse.up()
            assert page.locator('#pane-list').bounding_box()['height'] > list_box['height']+30
            folder_width = page.locator('#pane-folders').bounding_box()['width']
            page.locator('#folder-divider').focus()
            page.keyboard.press('ArrowRight')
            assert page.locator('#pane-folders').bounding_box()['width'] > folder_width
            bar_box = page.locator('#ruling-bar').bounding_box()
            pane_box = page.locator('#pane-reading').bounding_box()
            assert abs(bar_box['y']+bar_box['height']-pane_box['y']-pane_box['height']) < 2
            page.locator('#pane-list li').click()
            page.locator('#note-input').fill('Keep my reasoning while other work arrives.')
            page.locator('#note-input').focus()
            page.locator('#reading-content').evaluate('(el) => el.scrollTop = 160')
            original_scroll = page.locator('#reading-content').evaluate('(el) => el.scrollTop')
            write_json(feed / 'two.json', dict(hold, id='example-43-head', pr=43, title='Another new hold', held_at='2026-09-02T12:00:00Z'))
            expect(page.locator('#pane-list li')).to_have_count(2, timeout=15000)
            expect(page.locator('#pane-reading h1')).to_have_text(title)
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            expect(page.locator('#note-input')).to_be_focused()
            assert page.locator('#reading-content').evaluate('(el) => el.scrollTop') == original_scroll
            expect(page.locator('#activity-button')).to_have_text('Unread updates (0)')
            # A feed change is a data invalidation the operator can see.
            expect(page.locator('#cache-notice')).to_contain_text('Data cache rebuilt: version ' + version_before + ' -> ', timeout=15000)
            expect(page.locator('#cache-notice')).to_contain_text('feed changed')
            assert version_before not in page.locator('#data-status').inner_text()
            page.locator('#cache-notice-dismiss').click()
            if os.environ.get('HOLD_COURT_DOC_SCREENSHOTS'):
                page.locator('#reading-content').evaluate('(el) => el.scrollTop = 0')
                page.locator('#note-input').press('Escape')
                doc_screenshot('inbox.png')
                page.keyboard.press('?')
                expect(page.locator('#cheatsheet-overlay')).to_be_visible()
                doc_screenshot('keys.png')
                page.keyboard.press('Escape')
            hold['question'] = 'Updated review question'
            write_json(feed / 'one.json', hold)
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            expect(page.locator('.question')).to_have_text('Should this behavior change?')
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.locator('#show-update').click()
            expect(page.locator('.question')).to_have_text('Updated review question')
            page.locator('[data-action="discuss"]').click()
            page.locator('[data-action="discuss"]').click()
            expect(page.locator('#pending-bar')).to_be_hidden()
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.locator('[data-action="discuss"]').click()
            page.locator('#clear-ruling').click()
            expect(page.locator('#pending-bar')).to_be_hidden()
            page.locator('[data-action="discuss"]').click()
            page.reload()
            page.locator('[data-hold-id="example-42-head"]').click()
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            expect(page.locator('#pending-bar')).to_contain_text('1 pending ruling')
            page.route('**/api/rulings', lambda route: route.fulfill(status=503, body='temporarily unavailable'))
            page.locator('#save-btn').click()
            expect(page.locator('#notice')).to_contain_text('Save failed')
            expect(page.locator('#pending-bar')).to_contain_text('1 pending ruling')
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.unroute('**/api/rulings')
            activity_before = page.locator('#activity-button').inner_text()
            page.locator('#save-btn').click()
            expect(page.locator('#notice')).to_contain_text('Decisions saved')
            expect(page.locator('#latest-status')).to_contain_text('queued')
            expect(page.locator('#show-update')).to_be_hidden()
            expect(page.locator('#activity-button')).to_have_text(activity_before)
            assert 'Keep my reasoning' in confirmations[-1]
            assert len(list((tmp/'requests').glob('*.json'))) == 1
            expect(page.locator('#note-input')).to_have_value('')
            assert json.loads((rulings / 'example-42-head.json').read_text())['action'] == 'discuss'
            expect(page.locator('#pane-list li')).to_have_count(1)
            expect(page.locator('#pane-list [data-hold-id="example-42-head"]')).to_have_count(0)
            expect(page.locator('#pane-folders [data-folder-id="pending"]')).to_contain_text('1')
            write_json(rulings / 'example-42-head.result.json', {'status': 'failed', 'summary': 'Head changed. No action taken.'})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('#activity-button')).to_have_text('Unread updates (0)')
            # Reading the displayed update acknowledged it; later activity must return.
            write_json(rulings/'example-42-head.result.json', {'status':'in_progress', 'summary':'Agent acknowledged the request'})
            expect(page.locator('#activity-button')).to_have_text('Unread updates (1)', timeout=15000)
            page.reload()
            expect(page.locator('#activity-button')).to_have_text('Unread updates (1)')
            page.locator('#activity-button').click()
            expect(page.locator('#pane-list li')).to_have_count(1)
            page.locator('#pane-list [data-hold-id="example-42-head"]').click()
            expect(page.locator('#pane-list li')).to_have_count(0)
            expect(page.locator('#activity-button')).to_have_text('Unread updates (0)')
            expect(page.locator('#pane-reading h1')).to_have_text(title)
            page.locator('#note-input').fill('Keep the reply bound to this hold')
            page.locator('#pane-folders [data-folder-id="pending"]').click()
            expect(page.locator('#pane-list li')).to_have_count(1)
            expect(page.locator('#note-input')).to_have_value('Keep the reply bound to this hold')
            page.locator('#note-input').fill('')
            write_json(rulings/'example-42-head.result.json', {'status':'failed', 'summary':'Head changed. No action taken.'})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('#latest-status')).to_contain_text('Head changed. No action taken.')
            expect(page.locator('#latest-status')).to_contain_text('failed')
            page.locator('.history-tabs [data-tab="history"]').click()
            expect(page.locator('#reading-tab')).to_contain_text('Keep my reasoning')
            expect(page.locator('#reading-tab')).to_contain_text('Should this behavior change?')
            write_json(rulings/'example-42-head.result.json', {'status':'reply_ready', 'summary':'Agent reply is ready'})
            write_json(rulings/'example-42-head.thread.json', {'messages':[{'id':'reply-1','author':'test-agent','body':'Here is the reasoning you requested.','at':datetime.now(timezone.utc).isoformat()}]})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('#reading-tab')).to_contain_text('Here is the reasoning you requested.')
            if os.environ.get('HOLD_COURT_DOC_SCREENSHOTS'):
                page.locator('#reading-content').evaluate('(el) => el.scrollTop = 0')
                doc_screenshot('reading-pane.png')
            # A ruling is incomplete without a response to the hold, including Proceed.
            page.locator('#note-input').fill('')
            original_ruling = (rulings/'example-42-head.json').read_text()
            for action in ['proceed', 'changes', 'close', 'discuss']:
                page.locator('[data-action="'+action+'"]').click()
                page.locator('#save-btn').click()
                expect(page.locator('#notice')).to_contain_text('Respond to each hold before saving')
                assert (rulings/'example-42-head.json').read_text() == original_ruling
            page.locator('[data-action="close"]').click()
            page.locator('#note-input').fill('Close if superseded; clarify the replacement first.')
            page.locator('#save-btn').click()
            expect(page.locator('#latest-status')).to_contain_text('queued')
            saved = json.loads((rulings/'example-42-head.json').read_text())
            assert saved['action'] == 'close' and saved['note'] == 'Close if superseded; clarify the replacement first.'
            assert 'Annotations for the agent' in confirmations[-1]
            with page.expect_response(lambda response: '/api/holds' in response.url and response.status == 200
                                      and any((h.get('result') or {}).get('status') == 'needs_clarification'
                                              for h in response.json().get('holds', [])), timeout=15000):
                write_json(rulings/'example-42-head.result.json', {'ruling_id':saved['id'], 'status':'needs_clarification', 'summary':'What is the reason to close?'})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('#latest-status')).to_contain_text('needs clarification')
            expect(page.locator('#reading-tab')).to_contain_text('close')
            page.locator('#pane-folders [data-folder-id="inbox"]').click()
            for index in range(8):
                write_json(feed / f'more-{index}.json', dict(hold, id=f'more-{index}', pr=100+index,
                           title=f'Additional hold {index}', held_at='2026-09-03T12:00:00Z'))
            expect(page.locator('#pane-list li')).to_have_count(9, timeout=15000)
            page.locator('#pane-list').evaluate('(el) => el.scrollTop = 0')
            page.keyboard.press('G')
            assert page.locator('#pane-list').evaluate('(el) => el.scrollTop') > 0
            write_json(rulings/'example-42-head.result.json', {'ruling_id':saved['id'], 'status':'executed', 'summary':'Action completed'})
            expect(page.locator('#pane-folders [data-folder-id="pending"]')).to_contain_text('0', timeout=15000)
            expect(page.locator('#pane-folders [data-folder-id="executed"]')).to_contain_text('1')
            expect(page.locator('#activity-button')).to_have_text('Unread updates (1)')
            page.locator('#activity-button').click()
            page.locator('#pane-list [data-hold-id="example-42-head"]').click()
            expect(page.locator('#pane-list li')).to_have_count(0)
            expect(page.locator('#latest-status')).to_contain_text('Action completed')
            page.reload()
            expect(page.locator('#activity-button')).to_have_text('Unread updates (0)')
            assert not errors, errors
            if os.environ.get('HOLD_COURT_SCREENSHOT'):
                page.screenshot(path=os.environ['HOLD_COURT_SCREENSHOT'])
            browser.close()
        print('PASS: cached shell and rebuild, resize/docking, clearing choices, preserved context, confirmed queue handoff, no self-notification, version history and replies')
    finally:
        os.killpg(server.pid, signal.SIGINT)
        server.wait(timeout=15)
