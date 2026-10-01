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

        get ok() {
            const pass = Boolean(this.actual);
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + (this.isNegated ? " not to be truthy" : " to be truthy"));
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

        lengthOf(len) {
            const actualLen = this.actual ? this.actual.length : undefined;
            const pass = actualLen === len;
            const condition = this.isNegated ? !pass : pass;
            if (!condition) {
                throw new Error("expected " + formatValue(this.actual) + " to have length " + len + " but got " + actualLen);
            }
            return this;
        }
    }

    global.expect = function(actual) {
        return new Assertion(actual);
    };
})(this);
`
