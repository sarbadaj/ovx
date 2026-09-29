# This file is auto-generated from the current state of the database. Instead
# of editing this file, please use the migrations feature of Active Record to
# incrementally modify your database, and then regenerate this schema definition.
#
# This file is the source Rails uses to define your schema when running `bin/rails
# db:schema:load`. When creating a new database, `bin/rails db:schema:load` tends to
# be faster and is potentially less error prone than running all of your
# migrations from scratch. Old migrations may fail to apply correctly if those
# migrations use external dependencies or application code.
#
# It's strongly recommended that you check this file into your version control system.

ActiveRecord::Schema[8.1].define(version: 2026_09_29_000100) do
  # These are extensions that must be enabled in order to support this database
  enable_extension "pg_catalog.plpgsql"

  create_table "agent_events", force: :cascade do |t|
    t.bigint "task_id", null: false
    t.string "event_type", null: false
    t.jsonb "payload", default: {}, null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["task_id", "id"], name: "index_agent_events_on_task_id_and_id"
    t.index ["task_id"], name: "index_agent_events_on_task_id"
  end

  create_table "approvals", force: :cascade do |t|
    t.bigint "task_id", null: false
    t.bigint "tool_call_id", null: false
    t.string "status", default: "pending", null: false
    t.text "reason", null: false
    t.jsonb "requested_capabilities", default: [], null: false
    t.datetime "decided_at"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["status"], name: "index_approvals_on_status"
    t.index ["task_id"], name: "index_approvals_on_task_id"
    t.index ["tool_call_id"], name: "index_approvals_on_tool_call_id", unique: true
  end

  create_table "repositories", force: :cascade do |t|
    t.string "name", null: false
    t.string "path", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["path"], name: "index_repositories_on_path", unique: true
  end

  create_table "tasks", force: :cascade do |t|
    t.bigint "repository_id", null: false
    t.text "user_request", null: false
    t.string "state", default: "created", null: false
    t.string "workspace_path"
    t.string "base_revision"
    t.text "plan"
    t.text "final_message"
    t.text "error_message"
    t.integer "tool_calls_count", default: 0, null: false
    t.jsonb "transcript", default: [], null: false
    t.string "resolution"
    t.datetime "applied_at"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.string "workspace_token"
    t.index ["repository_id"], name: "index_tasks_on_repository_id"
    t.index ["state"], name: "index_tasks_on_state"
    t.index ["workspace_token"], name: "index_tasks_on_workspace_token", unique: true
  end

  create_table "tool_attempts", force: :cascade do |t|
    t.bigint "tool_call_id", null: false
    t.integer "attempt_number", default: 1, null: false
    t.string "state", null: false
    t.datetime "started_at"
    t.datetime "finished_at"
    t.integer "exit_code"
    t.jsonb "result", default: {}, null: false
    t.text "error_message"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["tool_call_id", "attempt_number"], name: "index_tool_attempts_on_tool_call_id_and_attempt_number", unique: true
    t.index ["tool_call_id"], name: "index_tool_attempts_on_tool_call_id"
  end

  create_table "tool_calls", force: :cascade do |t|
    t.bigint "task_id", null: false
    t.string "tool_name", null: false
    t.jsonb "arguments", default: {}, null: false
    t.string "input_hash"
    t.jsonb "required_capabilities", default: [], null: false
    t.string "state", default: "requested", null: false
    t.datetime "started_at"
    t.datetime "completed_at"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["task_id", "state"], name: "index_tool_calls_on_task_id_and_state"
    t.index ["task_id"], name: "index_tool_calls_on_task_id"
  end

  add_foreign_key "agent_events", "tasks"
  add_foreign_key "approvals", "tasks"
  add_foreign_key "approvals", "tool_calls"
  add_foreign_key "tasks", "repositories"
  add_foreign_key "tool_attempts", "tool_calls"
  add_foreign_key "tool_calls", "tasks"
end
