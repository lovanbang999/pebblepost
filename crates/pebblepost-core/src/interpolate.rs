use std::collections::HashMap;
use std::env;
use std::time::{SystemTime, UNIX_EPOCH};

use chrono::{SecondsFormat, Utc};
use serde_json::Value;

use crate::EnvironmentDefinition;

const MAX_DEPTH: usize = 10;
const ESCAPED_BRACES: &str = "\0_PEBBLE_ESCAPED_BRACE_\0";

pub fn build_variable_map(
    environment: Option<&EnvironmentDefinition>,
    overrides: &HashMap<String, String>,
) -> HashMap<String, String> {
    build_variable_map_with_folder(environment, None, overrides)
}

pub fn build_variable_map_with_folder(
    environment: Option<&EnvironmentDefinition>,
    folder_variables: Option<&serde_json::Map<String, Value>>,
    overrides: &HashMap<String, String>,
) -> HashMap<String, String> {
    let mut variables = HashMap::new();
    if let Some(environment) = environment {
        for variable in &environment.variables {
            if variable.enabled && !variable.key.is_empty() {
                variables.insert(variable.key.clone(), resolve_os_environment(&variable.value));
            }
        }
    }
    if let Some(folder_variables) = folder_variables {
        for (key, value) in folder_variables {
            if !key.is_empty() {
                let value = value.as_str().map(str::to_owned).unwrap_or_else(|| value.to_string());
                variables.insert(key.clone(), resolve_os_environment(&value));
            }
        }
    }
    for (key, value) in overrides {
        if !key.is_empty() {
            variables.insert(key.clone(), resolve_os_environment(value));
        }
    }
    variables
}

pub fn interpolate_string(input: &str, variables: &HashMap<String, String>) -> Result<String, String> {
    if input.is_empty() {
        return Ok(String::new());
    }
    let escaped = input.replace(r"\{{", ESCAPED_BRACES);
    resolve_with_chain(&escaped, variables, &mut Vec::new(), 0)
        .map(|value| value.replace(ESCAPED_BRACES, "{{"))
}

pub fn interpolate_value(value: &mut Value, variables: &HashMap<String, String>) -> Result<(), String> {
    match value {
        Value::String(text) => *text = interpolate_string(text, variables)?,
        Value::Array(items) => {
            for item in items {
                interpolate_value(item, variables)?;
            }
        }
        Value::Object(fields) => {
            for item in fields.values_mut() {
                interpolate_value(item, variables)?;
            }
        }
        _ => {}
    }
    Ok(())
}

fn resolve_with_chain(
    text: &str,
    variables: &HashMap<String, String>,
    chain: &mut Vec<String>,
    depth: usize,
) -> Result<String, String> {
    if !text.contains("{{") {
        return Ok(text.to_owned());
    }
    if depth >= MAX_DEPTH {
        return Err(format!("variable recursion depth exceeded maximum limit ({MAX_DEPTH}): {}", chain.join(" -> ")));
    }

    let mut current = text.to_owned();
    for _ in 0..MAX_DEPTH {
        let mut output = String::with_capacity(current.len());
        let mut cursor = 0;
        let mut changed = false;
        while let Some(relative_start) = current[cursor..].find("{{") {
            let start = cursor + relative_start;
            output.push_str(&current[cursor..start]);
            let value_start = start + 2;
            let Some(relative_end) = current[value_start..].find("}}") else {
                output.push_str(&current[start..]);
                cursor = current.len();
                break;
            };
            let end = value_start + relative_end;
            let key = current[value_start..end].trim();
            if !valid_key(key) {
                output.push_str(&current[start..end + 2]);
                cursor = end + 2;
                continue;
            }
            let replacement = if let Some(value) = variables.get(key) {
                if chain.iter().any(|previous| previous == key) {
                    let mut cycle = chain.clone();
                    cycle.push(key.to_owned());
                    return Err(format!("circular variable reference detected: {}", cycle.join(" -> ")));
                }
                chain.push(key.to_owned());
                let resolved = resolve_with_chain(value, variables, chain, depth + 1);
                chain.pop();
                resolved?
            } else if let Some(value) = resolve_dynamic_variable(key) {
                value
            } else {
                current[start..end + 2].to_owned()
            };
            if replacement != &current[start..end + 2] {
                changed = true;
            }
            output.push_str(&replacement);
            cursor = end + 2;
        }
        output.push_str(&current[cursor..]);
        if !changed || output == current {
            return Ok(output);
        }
        current = output;
    }
    Ok(current)
}

fn valid_key(key: &str) -> bool {
    !key.is_empty() && key.chars().all(|ch| ch.is_ascii_alphanumeric() || matches!(ch, '_' | '$' | '.' | '-'))
}

fn resolve_dynamic_variable(key: &str) -> Option<String> {
    let now = SystemTime::now().duration_since(UNIX_EPOCH).ok()?;
    match key.to_ascii_lowercase().as_str() {
        "$guid" | "$uuid" => random_uuid().ok(),
        "$timestamp" => Some(now.as_secs().to_string()),
        "$timestampms" => Some(now.as_millis().to_string()),
        "$isotimestamp" => Some(Utc::now().to_rfc3339_opts(SecondsFormat::Secs, true)),
        "$randomint" => random_number(1000).ok().map(|number| number.to_string()),
        "$randomemail" => random_number(10000).ok().map(|number| format!("user_{number}@example.com")),
        _ => None,
    }
}

fn random_number(max: u32) -> Result<u32, getrandom::Error> {
    let mut bytes = [0_u8; 4];
    getrandom::fill(&mut bytes)?;
    Ok(u32::from_ne_bytes(bytes) % max)
}

fn random_uuid() -> Result<String, getrandom::Error> {
    let mut bytes = [0_u8; 16];
    getrandom::fill(&mut bytes)?;
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    let hex = bytes.iter().map(|byte| format!("{byte:02x}")).collect::<String>();
    Ok(format!("{}-{}-{}-{}-{}", &hex[0..8], &hex[8..12], &hex[12..16], &hex[16..20], &hex[20..32]))
}

fn resolve_os_environment(value: &str) -> String {
    let mut output = String::with_capacity(value.len());
    let mut cursor = 0;
    while let Some(relative_start) = value[cursor..].find("${OS_ENV:") {
        let start = cursor + relative_start;
        output.push_str(&value[cursor..start]);
        let variable_start = start + "${OS_ENV:".len();
        let Some(relative_end) = value[variable_start..].find('}') else {
            output.push_str(&value[start..]);
            return output;
        };
        let end = variable_start + relative_end;
        let name = &value[variable_start..end];
        output.push_str(&env::var(name).unwrap_or_else(|_| value[start..end + 1].to_owned()));
        cursor = end + 1;
    }
    output.push_str(&value[cursor..]);
    output
}

/// Return the process environment values referenced by `${OS_ENV:NAME}`
/// placeholders. Callers use these values to prevent resolved secrets from
/// appearing in request logs and history.
pub fn resolved_os_environment_secrets(value: &str) -> Vec<String> {
    let mut secrets = Vec::new();
    let mut cursor = 0;
    while let Some(relative_start) = value[cursor..].find("${OS_ENV:") {
        let start = cursor + relative_start;
        let variable_start = start + "${OS_ENV:".len();
        let Some(relative_end) = value[variable_start..].find('}') else { break; };
        let end = variable_start + relative_end;
        let name = value[variable_start..end].trim();
        if let Ok(secret) = env::var(name) {
            if !secret.is_empty() {
                secrets.push(secret);
            }
        }
        cursor = end + 1;
    }
    secrets
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{EnvironmentDefinition, KeyValue};

    #[test]
    fn interpolation_resolves_nested_values_and_preserves_missing_and_escaped_tokens() {
        let variables = HashMap::from([
            ("HOST".to_owned(), "api.example.test".to_owned()),
            ("BASE".to_owned(), "https://{{HOST}}/v1".to_owned()),
            ("TOKEN".to_owned(), "abc".to_owned()),
        ]);
        assert_eq!(interpolate_string("{{BASE}}/users?token={{TOKEN}}", &variables).unwrap(), "https://api.example.test/v1/users?token=abc");
        assert_eq!(interpolate_string("{{MISSING}}", &variables).unwrap(), "{{MISSING}}");
        assert_eq!(interpolate_string(r"\{{TOKEN}}", &variables).unwrap(), "{{TOKEN}}");
        assert_eq!(interpolate_string("{{ .bad! }} {{a b}}", &variables).unwrap(), "{{ .bad! }} {{a b}}");
    }

    #[test]
    fn interpolation_reports_cycles_and_depth_exhaustion_without_panicking() {
        let cycle = HashMap::from([("A".into(), "{{B}}".into()), ("B".into(), "{{A}}".into())]);
        assert!(interpolate_string("{{A}}", &cycle).unwrap_err().contains("circular variable reference"));
        let long_chain = (0..12).map(|index| {
            let value = if index == 11 { "end".to_owned() } else { format!("{{{{V{}}}}}", index + 1) };
            (format!("V{index}"), value)
        }).collect::<HashMap<_, _>>();
        assert!(interpolate_string("{{V0}}", &long_chain).unwrap_err().contains("recursion depth"));
        for input in ["{{", "}}", "{{}}", "{{ }}", "{{VAR", "VAR}}", "{ { VAR } }"] {
            let _ = interpolate_string(input, &long_chain);
        }
    }

    #[test]
    fn variable_precedence_filters_disabled_values_and_interpolates_nested_json() {
        let environment = EnvironmentDefinition {
            variables: vec![
                KeyValue { key: "HOST".into(), value: "env.test".into(), enabled: true, ..KeyValue::default() },
                KeyValue { key: "DISABLED".into(), value: "skip".into(), enabled: false, ..KeyValue::default() },
            ],
            ..EnvironmentDefinition::default()
        };
        let folder = serde_json::from_value::<Value>(serde_json::json!({ "HOST": "folder.test", "FOLDER": 12 })).unwrap();
        let folder = folder.as_object().unwrap();
        let overrides = HashMap::from([("HOST".to_owned(), "runtime.test".to_owned())]);
        let variables = build_variable_map_with_folder(Some(&environment), Some(folder), &overrides);
        assert_eq!(variables.get("HOST").map(String::as_str), Some("runtime.test"));
        assert_eq!(variables.get("FOLDER").map(String::as_str), Some("12"));
        assert!(!variables.contains_key("DISABLED"));
        let mut value = serde_json::json!({ "url": "https://{{HOST}}/{{FOLDER}}", "values": ["{{UNKNOWN}}", 3] });
        interpolate_value(&mut value, &variables).unwrap();
        assert_eq!(value["url"], "https://runtime.test/12");
        assert_eq!(value["values"][0], "{{UNKNOWN}}");
    }

    #[test]
    fn dynamic_variables_return_expected_shapes() {
        let variables = HashMap::new();
        let uuid = interpolate_string("{{$uuid}}", &variables).unwrap();
        assert_eq!(uuid.len(), 36);
        assert_eq!(uuid.chars().filter(|ch| *ch == '-').count(), 4);
        let timestamp = interpolate_string("{{$timestamp}}", &variables).unwrap().parse::<u64>().unwrap();
        assert!(timestamp > 0);
        let email = interpolate_string("{{$randomemail}}", &variables).unwrap();
        assert!(email.starts_with("user_") && email.ends_with("@example.com"));
        let number = interpolate_string("{{$randomint}}", &variables).unwrap().parse::<u32>().unwrap();
        assert!(number < 1000);
    }
}
