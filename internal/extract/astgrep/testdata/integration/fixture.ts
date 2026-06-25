// Minimal TypeScript fixture for per-rule-file integration tests.
// Covers: ts-func, ts-class, ts-interface, ts-type-alias, ts-enum, ts-method, ts-decorator.

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
