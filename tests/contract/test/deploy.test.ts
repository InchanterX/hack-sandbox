// Static checks of the deployment catalogs and the rendered demo overlay.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { mustRun, parseYamlStream, readYaml, root } from './helpers.ts';

type Obj = { apiVersion: string; kind: string; metadata: { name: string; namespace?: string }; spec?: any };

const forbiddenKinds = new Set([
  'Secret',
  'Namespace',
  'ServiceAccount',
  'Role',
  'RoleBinding',
  'ClusterRole',
  'ClusterRoleBinding',
  'NetworkPolicy',
  'CiliumNetworkPolicy',
  'ResourceQuota',
  'LimitRange',
]);

function render(): Obj[] {
  return parseYamlStream(mustRun('kustomize', ['build', 'deploy/overlays/demo'], root)) as Obj[];
}

function containers(o: Obj): any[] {
  const pod = o.spec?.template?.spec;
  return pod ? [...(pod.initContainers ?? []), ...pod.containers] : [];
}

describe('deploy catalogs', () => {
  it('images.yaml builds exactly server and web', () => {
    const images = readYaml<{ images: unknown[] }>('deploy/images.yaml');
    assert.deepEqual(images.images, [
      { name: 'server', context: 'server', dockerfile: 'server/Dockerfile' },
      { name: 'web', context: 'web', dockerfile: 'web/Dockerfile' },
    ]);
  });

  it('secrets.yaml declares only db/POSTGRES_PASSWORD', () => {
    const secrets = readYaml<{ secrets: { name: string; keys: string[] }[] }>('deploy/secrets.yaml');
    assert.equal(secrets.secrets.length, 1);
    assert.equal(secrets.secrets[0]?.name, 'db');
    assert.deepEqual(secrets.secrets[0]?.keys, ['POSTGRES_PASSWORD']);
  });
});

describe('rendered demo overlay', () => {
  const objects = render();

  it('has no forbidden objects and everything lives in hack-sandbox', () => {
    for (const o of objects) {
      assert.ok(!forbiddenKinds.has(o.kind), `forbidden ${o.kind}/${o.metadata.name}`);
      assert.equal(o.metadata.namespace, 'hack-sandbox', `${o.kind}/${o.metadata.name}`);
    }
    const kinds = objects.map((o) => `${o.kind}/${o.metadata.name}`).sort();
    for (const want of ['ConfigMap', 'Deployment/server', 'Deployment/web', 'HTTPRoute', 'Service/db', 'Service/server', 'Service/web', 'StatefulSet/db']) {
      assert.ok(kinds.some((k) => k.startsWith(want)), `${want} missing in ${kinds.join(', ')}`);
    }
  });

  it('uses named images, limits and no forbidden settings', () => {
    for (const o of objects) {
      for (const c of containers(o)) {
        const where = `${o.kind}/${o.metadata.name}/${c.name}`;
        assert.ok(!String(c.image).endsWith(':latest'), `${where} uses latest`);
        assert.notEqual(c.imagePullPolicy, 'Always', where);
        assert.notEqual(c.securityContext?.privileged, true, where);
        for (const k of ['cpu', 'memory']) {
          assert.ok(c.resources?.requests?.[k], `${where} requests.${k}`);
          assert.ok(c.resources?.limits?.[k], `${where} limits.${k}`);
        }
      }
      for (const v of o.spec?.template?.spec?.volumes ?? []) {
        assert.equal(v.hostPath, undefined, `${o.kind}/${o.metadata.name} uses hostPath`);
      }
      if (o.kind === 'Service') {
        assert.ok(!['NodePort', 'LoadBalancer'].includes(o.spec.type), `Service/${o.metadata.name}`);
      }
    }
    const image = (name: string) => containers(objects.find((o) => o.kind === 'Deployment' && o.metadata.name === name)!)[0].image;
    assert.match(image('server'), /^server(:|@|$)|\/server(:|@|$)/);
    assert.match(image('web'), /^web(:|@|$)|\/web(:|@|$)/);
  });

  it('takes the database password only from secretKeyRef db/POSTGRES_PASSWORD', () => {
    for (const name of ['server', 'db']) {
      const o = objects.find((x) => x.metadata.name === name && ['Deployment', 'StatefulSet'].includes(x.kind));
      assert.ok(o, name);
      const env = containers(o).flatMap((c) => c.env ?? []);
      const pw = env.find((e: any) => e.name === 'POSTGRES_PASSWORD');
      assert.deepEqual(pw?.valueFrom?.secretKeyRef, { name: 'db', key: 'POSTGRES_PASSWORD' }, name);
    }
    const text = JSON.stringify(objects);
    assert.ok(!/postgres(ql)?:\/\/[^"]*:[^"@]*@/.test(text), 'no inline DSN with password');
  });

  it('routes /api to server and / to web through home-gateway', () => {
    const routes = objects.filter((o) => o.kind === 'HTTPRoute');
    assert.equal(routes.length, 1);
    const spec = routes[0]!.spec;
    assert.deepEqual(spec.parentRefs, [{ name: 'home-gateway', namespace: 'default' }]);
    assert.deepEqual(spec.hostnames, ['sandbox.hack.faberge-eggs.ru']);
    const rules = spec.rules.map((r: any) => ({
      path: r.matches[0].path,
      backend: r.backendRefs.map((b: any) => `${b.name}:${b.port}`),
      filters: r.filters ?? [],
    }));
    assert.deepEqual(rules, [
      { path: { type: 'PathPrefix', value: '/api' }, backend: ['server:8080'], filters: [] },
      { path: { type: 'PathPrefix', value: '/' }, backend: ['web:8080'], filters: [] },
    ]);
  });
});
