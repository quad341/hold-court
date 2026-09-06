import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('consumer', Path(__file__).with_name('consumer.py'))
consumer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(consumer)


class ConsumerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.config = {key:str(self.root / key) for key in ['spool','rulings','feed','city_root']}
        self.config.update(repos=['owner/repo'], target='mayor', issue_prefix='town', bd='bd', gc='gc', gh='gh')
        self.request = dict(id='a'*64, hold_id='test-hold', repo='owner/repo', pr=42,
                            head_sha='b'*40, action='discuss', note='Why is this held?',
                            ruled_by='operator', ruled_at='2026-09-05T12:00:00Z')
        Path(self.config['rulings']).mkdir()
        self.ruling = Path(self.config['rulings']) / 'test-hold.json'
        consumer.atomic(self.ruling, self.request)
        self.calls = []
        self.exists = False
        self.head = self.request['head_sha']
        self.bead = dict(status='open', updated_at='2026-09-05T12:01:00Z', metadata={})
        self.comments = []

    def fake_run(self, config, args, body=None):
        self.calls.append((args, body))
        if args[0] == 'gh': return {'state':'OPEN','headRefOid':self.head}
        if args[:2] == ['bd','show']:
            if not self.exists: raise subprocess.CalledProcessError(1,args)
            return [self.bead]
        if args[:2] == ['bd','create']:
            self.exists = True
            return {'id':'created'}
        if args[:2] == ['bd','comments']: return self.comments
        return {}

    def path(self):
        return Path(self.config['spool']) / (self.request['id']+'.json')

    def result(self):
        return consumer.read(Path(self.config['rulings']) / 'test-hold.result.json')

    def test_legacy_and_wrong_repository_rejected(self):
        for request in [dict(self.request,id=''),dict(self.request,repo='wrong/repo')]:
            with self.assertRaises(ValueError): consumer.enqueue(self.config,request)
        self.assertEqual(self.calls,[])

    def test_enqueue_retry_and_acknowledged_reply(self):
        consumer.enqueue(self.config,self.request)
        consumer.enqueue(self.config,self.request)
        self.assertEqual(len(list(Path(self.config['spool']).glob('*.json'))),1)
        self.assertEqual(self.result()['status'],'queued')
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.bead.update(status='in_progress',assignee='mayor')
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'in_progress')
        self.comments=[{'id':1,'text':'Here is the answer','author':'mayor','created_at':'2026-09-05T12:02:00Z'}]
        self.bead.update(status='closed',metadata={'holdcourt.outcome':'reply_ready'})
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'reply_ready')
        self.assertEqual(self.result()['thread'][0]['body'],'Here is the answer')
        self.assertEqual(sum(args[:2]==['gc','sling'] for args,_ in self.calls),1)
        self.assertEqual(sum(args[:2]==['bd','create'] for args,_ in self.calls),1)
        description=next(body for args,body in self.calls if args[:2]==['bd','create'])
        self.assertIn('analysis only',description)
        self.assertIn(self.request['head_sha'],description)

    def test_unclear_close_returns_clarification_and_preserves_intent(self):
        self.request.update(action='close', note='Close if this is superseded; clarify which replacement first.')
        consumer.atomic(self.ruling, self.request)
        consumer.enqueue(self.config, self.request)
        consumer.sync_job(self.config, self.path(), self.fake_run)
        create = next((args, body) for args, body in self.calls if args[:2] == ['bd', 'create'])
        self.assertEqual(create[0][create[0].index('--id')+1], 'town-hc-' + self.request['id'][:24])
        self.assertIn('Deliver text verbatim only when the operator explicitly asks', create[1])
        self.assertIn('pause execution and return to discussion', create[1])
        self.comments = [{'id':1, 'text':'What is the reason to close?', 'author':'mayor', 'created_at':'2026-09-05T12:02:00Z'}]
        self.bead.update(metadata={'holdcourt.outcome':'needs_clarification'})
        consumer.sync_job(self.config, self.path(), self.fake_run)
        self.assertEqual(self.result()['status'], 'needs_clarification')
        self.assertEqual(self.result()['thread'][0]['body'], 'What is the reason to close?')
        self.assertEqual(consumer.read(self.ruling), self.request)
        self.assertEqual(consumer.read(self.path())['request']['action'], 'close')

    def test_every_new_action_requires_response(self):
        for action in ['proceed', 'changes', 'close', 'discuss']:
            for note in ['', ' \n\t']:
                with self.subTest(action=action, note=note), self.assertRaises(ValueError):
                    consumer.enqueue(self.config, dict(self.request, action=action, note=note))
        self.assertFalse(Path(self.config['spool']).exists())

    def test_previously_queued_blank_request_can_still_be_observed(self):
        legacy = dict(self.request, note='')
        consumer.atomic(self.ruling, legacy)
        consumer.atomic(self.path(), {'request':legacy, 'routed':True, 'bead_id':'town-hc-legacy'})
        self.exists = True
        self.bead.update(metadata={'holdcourt.outcome':'needs_clarification'})
        consumer.sync_job(self.config, self.path(), self.fake_run)
        self.assertEqual(self.result()['status'], 'needs_clarification')

    def test_command_exposes_configured_tools_to_child_commands(self):
        config = dict(self.config, bd='/custom/beads/bin/bd', gc='/custom/gc/bin/gc', gh='/usr/bin/gh')
        with patch.dict(os.environ, {'PATH':'/usr/bin:/bin'}), patch('subprocess.run') as run:
            run.return_value.stdout = '{}'
            consumer.command(config, [config['gc'], 'sling', 'mayor', 'test'])
        environment = run.call_args.kwargs['env']
        self.assertEqual(environment['PATH'].split(os.pathsep)[:2], ['/custom/beads/bin', '/custom/gc/bin'])
        self.assertTrue(environment['PATH'].endswith('/usr/bin:/bin'))
        self.assertEqual(environment['GC_CITY_ROOT'], config['city_root'])

    def test_failed_dispatch_exposes_command_diagnostic(self):
        consumer.enqueue(self.config, self.request)
        failure = subprocess.CalledProcessError(1, ['gc', 'sling'], stderr='target unavailable')
        with patch.object(consumer, 'sync_job', side_effect=failure), patch('sys.stderr'):
            consumer.worker(self.config)
        self.assertIn('target unavailable', self.result()['summary'])
        self.assertIn('target unavailable', consumer.read(self.path())['last_error'])

    def test_stale_head_never_dispatches(self):
        consumer.enqueue(self.config,self.request)
        self.head='c'*40
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'needs_decision')
        self.assertFalse(any(args[0]=='gc' for args,_ in self.calls))

    def test_superseded_unsent_decision_does_not_dispatch(self):
        consumer.enqueue(self.config,self.request)
        consumer.atomic(self.ruling,dict(self.request,id='d'*64))
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.calls,[])

    def test_old_reply_preserved_without_overwriting_new_result(self):
        consumer.enqueue(self.config,self.request)
        consumer.sync_job(self.config,self.path(),self.fake_run)
        newer=dict(self.request,id='d'*64)
        consumer.atomic(self.ruling,newer)
        consumer.enqueue(self.config,newer)
        self.comments=[{'id':1,'text':'Late reply','author':'mayor','created_at':'2026-09-05T12:02:00Z'}]
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['ruling_id'],newer['id'])
        thread=consumer.read(Path(self.config['rulings'])/'test-hold.thread.json')
        self.assertEqual(thread['messages'][0]['body'],'Late reply')

    def test_closed_task_without_outcome_is_not_execution(self):
        consumer.enqueue(self.config,self.request)
        self.bead['status']='closed'
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'needs_decision')


if __name__ == '__main__': unittest.main()
