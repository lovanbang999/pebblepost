package scripting

const AssertionLibraryJS = `
(function(global) {
    function formatValue(v) {
        if (v === null) return 'null';
        if (v === undefined) return 'undefined';
        if (typeof v === 'string') return JSON.stringify(v);
        if (typeof v === 'object') {
            try { return JSON.stringify(v); } catch(e) { return String(v); }
        }
        return String(v);
    }

    function deepEqual(a, b) {
        if (a === b) return true;
        if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object') return false;
        if (Array.isArray(a) !== Array.isArray(b)) return false;

        const keysA = Object.keys(a);
        const keysB = Object.keys(b);
        if (keysA.length !== keysB.length) return false;

        for (let i = 0; i < keysA.length; i++) {
            const key = keysA[i];
            if (!Object.prototype.hasOwnProperty.call(b, key)) return false;
            if (!deepEqual(a[key], b[key])) return false;
        }
        return true;
    }

    function validateJSONSchema(data, schema, path, errors) {
        if (!schema || typeof schema !== 'object') return;
        path = path || 'root';

        // 1. type
        if (schema.type) {
            const types = Array.isArray(schema.type) ? schema.type : [schema.type];
            let matched = false;
            for (let i = 0; i < types.length; i++) {
                const t = types[i].toLowerCase();
                if (t === 'null' && data === null) matched = true;
                else if (t === 'array' && Array.isArray(data)) matched = true;
                else if (t === 'integer' && typeof data === 'number' && Math.floor(data) === data) matched = true;
                else if (t === 'number' && typeof data === 'number') matched = true;
                else if (t === 'boolean' && typeof data === 'boolean') matched = true;
                else if (t === 'string' && typeof data === 'string') matched = true;
                else if (t === 'object' && typeof data === 'object' && data !== null && !Array.isArray(data)) matched = true;
            }
            if (!matched) {
                errors.push(path + ": expected type " + JSON.stringify(schema.type) + ", got " + (data === null ? 'null' : Array.isArray(data) ? 'array' : typeof data));
                return; // skip further checks on wrong type
            }
        }

        // 2. enum
        if (Array.isArray(schema.enum)) {
            let inEnum = false;
            for (let i = 0; i < schema.enum.length; i++) {
                if (deepEqual(data, schema.enum[i])) {
                    inEnum = true;
                    break;
                }
            }
            if (!inEnum) {
                errors.push(path + ": value " + formatValue(data) + " is not in enum " + JSON.stringify(schema.enum));
            }
        }

        // 3. Object checks
        if (typeof data === 'object' && data !== null && !Array.isArray(data)) {
            // required
            if (Array.isArray(schema.required)) {
                for (let i = 0; i < schema.required.length; i++) {
                    const reqProp = schema.required[i];
                    if (!Object.prototype.hasOwnProperty.call(data, reqProp)) {
                        errors.push(path + ": missing required property '" + reqProp + "'");
                    }
                }
            }

            // properties
            if (schema.properties && typeof schema.properties === 'object') {
                const propKeys = Object.keys(schema.properties);
                for (let i = 0; i < propKeys.length; i++) {
                    const prop = propKeys[i];
                    if (Object.prototype.hasOwnProperty.call(data, prop)) {
                        validateJSONSchema(data[prop], schema.properties[prop], path + "." + prop, errors);
                    }
                }
            }
        }

        // 4. Array checks
        if (Array.isArray(data)) {
            if (schema.items) {
                for (let i = 0; i < data.length; i++) {
                    validateJSONSchema(data[i], schema.items, path + "[" + i + "]", errors);
                }
            }
            if (typeof schema.minItems === 'number' && data.length < schema.minItems) {
                errors.push(path + ": array length " + data.length + " is less than minItems " + schema.minItems);
            }
            if (typeof schema.maxItems === 'number' && data.length > schema.maxItems) {
                errors.push(path + ": array length " + data.length + " exceeds maxItems " + schema.maxItems);
            }
        }

        // 5. String checks
        if (typeof data === 'string') {
            if (typeof schema.minLength === 'number' && data.length < schema.minLength) {
                errors.push(path + ": string length " + data.length + " is less than minLength " + schema.minLength);
            }
            if (typeof schema.maxLength === 'number' && data.length > schema.maxLength) {
                errors.push(path + ": string length " + data.length + " exceeds maxLength " + schema.maxLength);
            }
            if (schema.pattern) {
                const reg = new RegExp(schema.pattern);
                if (!reg.test(data)) {
                    errors.push(path + ": string does not match pattern '" + schema.pattern + "'");
                }
            }
        }

        // 6. Number checks
        if (typeof data === 'number') {
            if (typeof schema.minimum === 'number' && data < schema.minimum) {
                errors.push(path + ": value " + data + " is less than minimum " + schema.minimum);
            }
            if (typeof schema.maximum === 'number' && data > schema.maximum) {
                errors.push(path + ": value " + data + " exceeds maximum " + schema.maximum);
            }
        }
    }

    class Assertion {
        constructor(actual, isNegated = false) {
            this.actual = actual;
            this.isNegated = isNegated;
        }

        get not() {
            return new Assertion(this.actual, !this.isNegated);
        }

        get to() {
            return this;
        }

        get be() {
            return this;
        }

        get have() {
            return this;
        }

        get is() {
            return this;
        }

        get that() {
            return this;
        }

        get which() {
            return this;
        }

        get deep() {
            return this;
        }

        get ok() {
            const pass = Boolean(this.actual);
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be truthy" : " to be truthy"));
            }
            return this;
        }

        get exist() {
            const pass = this.actual !== null && this.actual !== undefined;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to exist" : " to exist"));
            }
            return this;
        }

        get true() {
            const pass = this.actual === true;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be true" : " to be true"));
            }
            return this;
        }

        get false() {
            const pass = this.actual === false;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be false" : " to be false"));
            }
            return this;
        }

        get null() {
            const pass = this.actual === null;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be null" : " to be null"));
            }
            return this;
        }

        get undefined() {
            const pass = this.actual === undefined;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be undefined" : " to be undefined"));
            }
            return this;
        }

        equal(expected) {
            const pass = this.actual === expected || deepEqual(this.actual, expected);
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to equal " : " to equal ") + formatValue(expected));
            }
            return this;
        }

        eq(expected) {
            return this.equal(expected);
        }

        eql(expected) {
            return this.equal(expected);
        }

        status(expected) {
            let actualStatus = this.actual;
            if (this.actual && typeof this.actual === 'object' && typeof this.actual.status === 'number') {
                actualStatus = this.actual.status;
            } else if (this.actual && typeof this.actual === 'object' && typeof this.actual.code === 'number') {
                actualStatus = this.actual.code;
            }
            const pass = actualStatus === expected;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected response status " + (this.isNegated ? "not to equal " : "to equal ") + expected + " but got " + actualStatus);
            }
            return this;
        }

        a(typeStr) {
            let actualType = typeof this.actual;
            if (this.actual === null) actualType = 'null';
            else if (Array.isArray(this.actual)) actualType = 'array';

            const pass = actualType.toLowerCase() === String(typeStr).toLowerCase();
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be a " : " to be a ") + typeStr + " (got " + actualType + ")");
            }
            return this;
        }

        an(typeStr) {
            return this.a(typeStr);
        }

        property(key, value) {
            const hasProp = this.actual !== null && this.actual !== undefined && Object.prototype.hasOwnProperty.call(this.actual, key);
            if (!this.isNegated) {
                if (!hasProp) {
                    throw new Error("expected " + formatValue(this.actual) + " to have property '" + key + "'");
                }
                if (arguments.length > 1) {
                    const valMatch = this.actual[key] === value || deepEqual(this.actual[key], value);
                    if (!valMatch) {
                        throw new Error("expected property '" + key + "' of " + formatValue(this.actual) + " to equal " + formatValue(value) + " but got " + formatValue(this.actual[key]));
                    }
                }
            } else {
                if (hasProp && arguments.length === 1) {
                    throw new Error("expected " + formatValue(this.actual) + " not to have property '" + key + "'");
                }
                if (hasProp && arguments.length > 1) {
                    const valMatch = this.actual[key] === value || deepEqual(this.actual[key], value);
                    if (valMatch) {
                        throw new Error("expected property '" + key + "' not to equal " + formatValue(value));
                    }
                }
            }
            return this;
        }

        include(item) {
            let pass = false;
            if (typeof this.actual === 'string') {
                pass = this.actual.includes(String(item));
            } else if (Array.isArray(this.actual)) {
                pass = this.actual.some(x => x === item || deepEqual(x, item));
            } else if (this.actual && typeof this.actual === 'object') {
                pass = Object.prototype.hasOwnProperty.call(this.actual, item);
            }
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to include " : " to include ") + formatValue(item));
            }
            return this;
        }

        contain(item) {
            return this.include(item);
        }

        above(num) {
            const pass = this.actual > num;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be above " : " to be above ") + num);
            }
            return this;
        }

        greaterThan(num) {
            return this.above(num);
        }

        atLeast(num) {
            const pass = this.actual >= num;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be at least " : " to be at least ") + num);
            }
            return this;
        }

        below(num) {
            const pass = this.actual < num;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be below " : " to be below ") + num);
            }
            return this;
        }

        lessThan(num) {
            return this.below(num);
        }

        atMost(num) {
            const pass = this.actual <= num;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be at most " : " to be at most ") + num);
            }
            return this;
        }

        length(len) {
            const actualLen = this.actual !== null && this.actual !== undefined ? this.actual.length : undefined;
            const pass = actualLen === len;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to have length " : " to have length ") + len + " but got " + actualLen);
            }
            return this;
        }

        lengthOf(len) {
            return this.length(len);
        }

        oneOf(list) {
            const pass = Array.isArray(list) && list.some(item => this.actual === item || deepEqual(this.actual, item));
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be one of " : " to be one of ") + formatValue(list));
            }
            return this;
        }

        match(pattern) {
            let pass = false;
            if (pattern instanceof RegExp) {
                pass = pattern.test(String(this.actual));
            } else {
                pass = new RegExp(String(pattern)).test(String(this.actual));
            }
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to match " : " to match ") + String(pattern));
            }
            return this;
        }

        jsonSchema(schema) {
            const errors = [];
            validateJSONSchema(this.actual, schema, 'root', errors);
            const pass = errors.length === 0;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                if (this.isNegated) {
                    throw new Error("expected data not to match JSON schema");
                }
                throw new Error("JSON schema validation failed:\n  • " + errors.join("\n  • "));
            }
            return this;
        }

        matchSchema(schema) {
            return this.jsonSchema(schema);
        }
    }

    global.expect = function(actual) {
        return new Assertion(actual);
    };
})(this);
`
