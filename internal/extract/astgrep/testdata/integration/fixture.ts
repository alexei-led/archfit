// Minimal TypeScript fixture for per-rule-file integration tests.
// Covers: ts-func, ts-class, ts-interface, ts-type-alias, ts-enum, ts-method, ts-decorator,
//         ts-route-express (confirmed via ts-import-express signal),
//         ts-test-import-jest (test_import fact),
//         ts-test-fn (it()/test() call expressions → test_fn kind).

import express from 'express';
import { expect } from '@jest/globals';

export function greet(name: string): string {
    return `Hello, ${name}`;
}

export class Greeter {
    sayHello(): string {
        return "hello";
    }
}

export interface Salutation {
    message: string;
}

export type Greeting = string;

export enum Direction {
    Up,
    Down,
}

@Controller("/api")
export class ApiController {}

const app = express();
app.get('/hello', (req, res) => { res.send('hello'); });

// Exercises ts-test-fn: it()/test() call expressions emit test_fn facts.
// describe() is a suite, not a test, and must NOT match.
describe('greet', () => {
    it('greets by name', () => {
        expect(greet('world')).toBe('Hello, world');
    });
    test('handles empty name', () => {
        expect(greet('')).toBe('Hello, ');
    });
});
